#!/usr/bin/env bash
# benchmark.sh — sentra vs docker, startup latency + idle-container memory.
#
# Runs INSIDE WSL2 / Linux (needs runc, and docker for the comparison side).
#   ./scripts/benchmark.sh [--sentra BIN] [--image alpine:3.20]
#                          [--runs 20] [--mem-runs 5] [--out benchmarks/results]
#                          [--only-sentra]
#
# Method (documented so the numbers are defensible):
#   startup: wall-clock of a full CLI invocation running /bin/true —
#            `sentra run --rootfs <pulled> -- /bin/true` vs
#            `docker run --rm <image> /bin/true`. Same pinned image, same
#            command. One cold run each after drop_caches (needs privilege;
#            skipped with a note otherwise), then N warm runs interleaved
#            AB/BA per round to cancel drift.
#   memory:  detached `/bin/sleep 300`, wait 2s, read VmHWM (peak RSS) of
#            the container init PID from /proc — identical method for both
#            runtimes, no cgroup-path guessing, works rootless.
# Output: <out>/<stamp>.json (env, versions, raw samples) + <stamp>.md
#         (Markdown table for the README) printed to stdout too.
set -u

IMAGE="alpine:3.20"
RUNS=20
MEM_RUNS=5
ONLY_SENTRA=0
SENTRA=""
OUT=""

usage() {
	echo "usage: $0 [--sentra BIN] [--image IMG] [--runs N] [--mem-runs M] [--out DIR] [--only-sentra]" >&2
	exit 2
}

while [ $# -gt 0 ]; do
	case "$1" in
		--sentra) SENTRA="$2"; shift 2 ;;
		--image) IMAGE="$2"; shift 2 ;;
		--runs) RUNS="$2"; shift 2 ;;
		--mem-runs) MEM_RUNS="$2"; shift 2 ;;
		--out) OUT="$2"; shift 2 ;;
		--only-sentra) ONLY_SENTRA=1; shift ;;
		-h|--help) usage ;;
		*) echo "unknown flag $1" >&2; usage ;;
	esac
done

REPO="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${OUT:-$REPO/benchmarks/results}"
mkdir -p "$OUT"

if [ -z "$SENTRA" ]; then
	if command -v sentra >/dev/null 2>&1; then SENTRA="$(command -v sentra)"
	elif [ -x /usr/local/bin/sentra ]; then SENTRA=/usr/local/bin/sentra
	else SENTRA="$REPO/bin/sentra-linux"; fi
fi
[ -x "$SENTRA" ] || { echo "sentra binary not found/executable: $SENTRA" >&2; exit 1; }
command -v runc >/dev/null 2>&1 || { echo "runc not found in PATH" >&2; exit 1; }
if [ "$ONLY_SENTRA" -eq 0 ]; then
	docker info >/dev/null 2>&1 || {
		echo "docker daemon unreachable. Enable Docker Desktop WSL integration" >&2
		echo "for this distro, or re-run with --only-sentra for sentra-only numbers." >&2
		exit 1
	}
fi

# Rootless runc keeps state under XDG_RUNTIME_DIR; ensure it exists so both
# sentra and our `runc state` probe see the same view.
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
mkdir -p "$XDG_RUNTIME_DIR" 2>/dev/null || export XDG_RUNTIME_DIR="/tmp/sentra-bench-runtime-$$"

echo "==> pulling $IMAGE (both runtimes share the pinned image)"
[ "$ONLY_SENTRA" -eq 0 ] && docker pull "$IMAGE" >/dev/null
PULL_OUT="$("$SENTRA" pull "$IMAGE" 2>&1)" || { echo "$PULL_OUT" >&2; echo "sentra pull failed" >&2; exit 1; }
echo "$PULL_OUT" | tail -3
ROOTFS="$(echo "$PULL_OUT" | grep -o -- '--rootfs [^ ]*' | head -1 | cut -d' ' -f2)"
[ -n "$ROOTFS" ] || { echo "could not parse rootfs from sentra pull output" >&2; exit 1; }
[ -d "$ROOTFS" ] || { echo "rootfs dir missing: $ROOTFS" >&2; exit 1; }
DIGEST=""
[ "$ONLY_SENTRA" -eq 0 ] && DIGEST="$(docker inspect --format='{{join .RepoDigests ","}}' "$IMAGE" 2>/dev/null)"

STAMP="$(date +%Y%m%d-%H%M%S)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# now_ms: monotonic wall clock in milliseconds (date +%s%N -> ms).
now_ms() { date +%s%N | cut -b1-13; }

can_drop_caches() {
	[ -w /proc/sys/vm/drop_caches ] && return 0
	sudo -n true 2>/dev/null
}
drop_caches() {
	if [ -w /proc/sys/vm/drop_caches ]; then echo 3 > /proc/sys/vm/drop_caches; sync
	elif sudo -n true 2>/dev/null; then sudo sh -c 'sync; echo 3 > /proc/sys/vm/drop_caches'
	else return 1; fi
}

# time_run <outfile> -- <cmd...>: append one wall-clock sample (ms).
time_run() {
	local out="$1"; shift
	[ "$1" = "--" ] && shift
	local s e rc
	s=$(now_ms)
	"$@" >/dev/null 2>&1
	rc=$?
	e=$(now_ms)
	if [ $rc -ne 0 ]; then echo "FAILED (rc=$rc): $*" >&2; return 1; fi
	echo $((e - s)) >> "$out"
}

echo "==> warmup (3 untimed runs each)"
for _ in 1 2 3; do
	"$SENTRA" run --rootfs "$ROOTFS" -- /bin/true >/dev/null 2>&1
	[ "$ONLY_SENTRA" -eq 0 ] && docker run --rm "$IMAGE" /bin/true >/dev/null 2>&1
done

COLD_SENTRA="null"; COLD_DOCKER="null"; COLD_NOTE=""
if can_drop_caches; then
	echo "==> cold runs (page cache dropped)"
	drop_caches && time_run "$TMP/cold_s" -- "$SENTRA" run --rootfs "$ROOTFS" -- /bin/true \
		&& COLD_SENTRA="$(cat "$TMP/cold_s")"
	if [ "$ONLY_SENTRA" -eq 0 ]; then
		drop_caches && time_run "$TMP/cold_d" -- docker run --rm "$IMAGE" /bin/true \
			&& COLD_DOCKER="$(cat "$TMP/cold_d")"
	fi
else
	COLD_NOTE="drop_caches needs privilege; cold runs skipped (run script with sudo-capable user for them)"
	echo "==> cold runs skipped ($COLD_NOTE)"
fi

echo "==> startup: $RUNS warm runs each, interleaved"
: > "$TMP/sentra.startup"; : > "$TMP/docker.startup"
i=0
while [ $i -lt "$RUNS" ]; do
	i=$((i + 1))
	if [ $((i % 2)) -eq 1 ]; then
		time_run "$TMP/sentra.startup" -- "$SENTRA" run --rootfs "$ROOTFS" -- /bin/true || exit 1
		[ "$ONLY_SENTRA" -eq 0 ] && { time_run "$TMP/docker.startup" -- docker run --rm "$IMAGE" /bin/true || exit 1; }
	else
		[ "$ONLY_SENTRA" -eq 0 ] && { time_run "$TMP/docker.startup" -- docker run --rm "$IMAGE" /bin/true || exit 1; }
		time_run "$TMP/sentra.startup" -- "$SENTRA" run --rootfs "$ROOTFS" -- /bin/true || exit 1
	fi
done

# mem_hwm <pid>: peak RSS of a process in kB (VmHWM from /proc).
mem_hwm() { awk '/^VmHWM:/ {print $2}' "/proc/$1/status"; }

echo "==> memory: $MEM_RUNS detached sleepers each (VmHWM of init PID)"
: > "$TMP/sentra.mem"; : > "$TMP/docker.mem"
i=0
while [ $i -lt "$MEM_RUNS" ]; do
	i=$((i + 1))
	n="bench-s-$i-$$"
	"$SENTRA" run -d --name "$n" --rootfs "$ROOTFS" -- /bin/sleep 300 >/dev/null 2>&1 || { echo "sentra detached run failed" >&2; exit 1; }
	sleep 2
	pid="$(runc --rootless=true state "$n" 2>/dev/null | grep -o '"pid": [0-9]*' | grep -o '[0-9]*')"
	[ -n "$pid" ] || pid="$(runc state "$n" 2>/dev/null | grep -o '"pid": [0-9]*' | grep -o '[0-9]*')"
	[ -n "$pid" ] || { echo "could not get PID for $n" >&2; "$SENTRA" stop "$n" >/dev/null 2>&1; "$SENTRA" rm "$n" >/dev/null 2>&1; exit 1; }
	mem_hwm "$pid" >> "$TMP/sentra.mem"
	"$SENTRA" stop "$n" >/dev/null 2>&1; "$SENTRA" rm "$n" >/dev/null 2>&1
	if [ "$ONLY_SENTRA" -eq 0 ]; then
		dn="bench-d-$i-$$"
		docker run -d --name "$dn" "$IMAGE" sleep 300 >/dev/null 2>&1 || { echo "docker detached run failed" >&2; exit 1; }
		sleep 2
		dpid="$(docker inspect --format='{{.State.Pid}}' "$dn")"
		mem_hwm "$dpid" >> "$TMP/docker.mem"
		docker rm -f "$dn" >/dev/null 2>&1
	fi
done

# stats <file>: count mean median min max p95 (all ms or kB integers).
stats() {
	awk '{
		a[n++]=$1; s+=$1;
		if (n==1 || $1<min) min=$1;
		if (n==1 || $1>max) max=$1;
	}
	END {
		if (n==0) { print "0 0 0 0 0 0"; exit }
		# insertion sort (n is small)
		for (i=1;i<n;i++) { v=a[i]; j=i-1; while (j>=0 && a[j]>v) { a[j+1]=a[j]; j-- } a[j+1]=v }
		med = (n%2==1) ? a[int(n/2)] : int((a[n/2-1]+a[n/2])/2);
		p = a[int(0.95*(n-1))];
		printf "%d %d %d %d %d %d", n, int(s/n), med, min, max, p;
	}' "$1"
}

read -r SN SMean SMed SMin SMax SP95 <<< "$(stats "$TMP/sentra.startup")"
read -r SMN SMMean SMMed SMMin SMMax SMP95 <<< "$(stats "$TMP/sentra.mem")"
if [ "$ONLY_SENTRA" -eq 0 ]; then
	read -r DN DMean DMed DMin DMax DP95 <<< "$(stats "$TMP/docker.startup")"
	read -r DMN DMMean DMMed DMMin DMMax DMP95 <<< "$(stats "$TMP/docker.mem")"
else
	DN=0; DMean=0; DMed=0; DMin=0; DMax=0; DP95=0
	DMN=0; DMMean=0; DMMed=0; DMMin=0; DMMax=0; DMP95=0
fi

KERNEL="$(uname -r)"; CPUS="$(nproc)"; MEMKB="$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)"
RUNC_V="$(runc --version 2>/dev/null | head -1)"
DOCKER_V=""; [ "$ONLY_SENTRA" -eq 0 ] && DOCKER_V="$(docker --version 2>/dev/null)"
SENTRA_SHA="$(git -C "$REPO" rev-parse --short HEAD 2>/dev/null || echo unknown)"
EUID_NOW="$(id -u)"

# json_array <file>: [1, 2, 3] (or [] when there are no samples).
json_array() {
	local s
	s=$(tr '\n' ',' < "$1" | sed 's/,$//')
	if [ -z "$s" ]; then echo "[]"; else echo "[$s]"; fi
}

JSON="$OUT/$STAMP.json"
{
echo "{"
echo "  \"stamp\": \"$STAMP\","
echo "  \"image\": \"$IMAGE\", \"image_digest\": \"$DIGEST\","
echo "  \"env\": {\"kernel\": \"$KERNEL\", \"cpus\": $CPUS, \"mem_kb\": $MEMKB, \"euid\": $EUID_NOW},"
echo "  \"versions\": {\"runc\": \"$RUNC_V\", \"docker\": \"$DOCKER_V\", \"sentra_sha\": \"$SENTRA_SHA\"},"
echo "  \"method\": {\"startup\": \"wall-clock ms of full CLI run of /bin/true; 1 cold run after drop_caches + $RUNS warm interleaved\", \"memory\": \"VmHWM kB of detached /bin/sleep 300 init PID after 2s settle\"},"
echo "  \"cold_note\": \"$COLD_NOTE\","
echo "  \"startup_ms\": {"
echo "    \"sentra\": {\"cold\": $COLD_SENTRA, \"n\": $SN, \"mean\": $SMean, \"median\": $SMed, \"min\": $SMin, \"max\": $SMax, \"p95\": $SP95, \"samples\": $(json_array "$TMP/sentra.startup")},"
echo "    \"docker\": {\"cold\": $COLD_DOCKER, \"n\": $DN, \"mean\": $DMean, \"median\": $DMed, \"min\": $DMin, \"max\": $DMax, \"p95\": $DP95, \"samples\": $(json_array "$TMP/docker.startup")}"
echo "  },"
echo "  \"memory_kb\": {"
echo "    \"sentra\": {\"n\": $SMN, \"mean\": $SMMean, \"median\": $SMMed, \"min\": $SMMin, \"max\": $SMMax, \"p95\": $SMP95, \"samples\": $(json_array "$TMP/sentra.mem")},"
echo "    \"docker\": {\"n\": $DMN, \"mean\": $DMMean, \"median\": $DMMed, \"min\": $DMMin, \"max\": $DMMax, \"p95\": $DMP95, \"samples\": $(json_array "$TMP/docker.mem")}"
echo "  }"
echo "}"
} > "$JSON"

MD="$OUT/$STAMP.md"
cold_fmt() { [ "$1" = "null" ] && echo "skipped" || echo "$1ms"; }
{
echo "# Benchmark $STAMP — \`$IMAGE\` ($KERNEL, ${CPUS} CPU)"
echo
echo "sentra @ $SENTRA_SHA · $RUNC_V · ${DOCKER_V:-docker skipped}"
echo
echo "| metric | sentra | docker |"
echo "|---|---|---|"
if [ "$ONLY_SENTRA" -eq 0 ]; then
echo "| startup median (mean/p95, $SN runs) | ${SMed}ms (${SMean}/${SP95}) | ${DMed}ms (${DMean}/${DP95}) |"
echo "| startup cold (drop_caches) | $(cold_fmt "$COLD_SENTRA") | $(cold_fmt "$COLD_DOCKER") |"
echo "| idle VmHWM median ($SMN runs) | ${SMMed}kB | ${DMMed}kB |"
else
echo "| startup median (mean/p95, $SN runs) | ${SMed}ms (${SMean}/${SP95}) | — |"
echo "| startup cold (drop_caches) | $(cold_fmt "$COLD_SENTRA") | — |"
echo "| idle VmHWM median ($SMN runs) | ${SMMed}kB | — |"
fi
[ -n "$COLD_NOTE" ] && echo -e "\n> $COLD_NOTE"
} > "$MD"

cat "$MD"
echo
echo "raw JSON: $JSON"
