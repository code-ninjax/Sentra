import os
from PIL import Image, ImageDraw, ImageFont

def generate_og_image():
    width = 1200
    height = 630
    
    # Create high-res RGB canvas
    img = Image.new("RGB", (width, height), color=(10, 10, 10))
    draw = ImageDraw.Draw(img)
    
    public_dir = os.path.join(os.path.dirname(__file__), "..", "website", "public")
    font_bold_path = os.path.join(public_dir, "AeonikTRIAL-Bold.otf")
    font_reg_path = os.path.join(public_dir, "AeonikTRIAL-Regular.otf")
    
    # Load fonts
    font_logo = ImageFont.truetype(font_bold_path, 42)
    font_title = ImageFont.truetype(font_bold_path, 58)
    font_subtitle = ImageFont.truetype(font_reg_path, 25)
    font_badge = ImageFont.truetype(font_bold_path, 15)
    font_domain = ImageFont.truetype(font_bold_path, 22)
    
    # Monospace font for terminal box
    mono_font_path = "C:/Windows/Fonts/consola.ttf"
    mono_bold_path = "C:/Windows/Fonts/consolab.ttf"
    if os.path.exists(mono_font_path):
        font_term = ImageFont.truetype(mono_font_path, 21)
        font_term_meta = ImageFont.truetype(mono_font_path, 15)
    else:
        font_term = ImageFont.load_default()
        font_term_meta = ImageFont.load_default()

    # Draw subtle background grid
    grid_color = (24, 24, 24)
    for x in range(0, width, 40):
        draw.line([(x, 0), (x, height)], fill=grid_color, width=1)
    for y in range(0, height, 40):
        draw.line([(0, y), (width, y)], fill=grid_color, width=1)

    # Accent color: #4ADE80
    accent_rgb = (74, 222, 128)

    top_y = 66
    left_x = 90

    # Draw logo icon: sharp square with green dot
    icon_size = 46
    draw.rectangle([left_x, top_y, left_x + icon_size, top_y + icon_size], outline=accent_rgb, width=3)
    dot_r = 7
    cx = left_x + icon_size // 2
    cy = top_y + icon_size // 2
    draw.ellipse([cx - dot_r, cy - dot_r, cx + dot_r, cy + dot_r], fill=accent_rgb)

    # Brand text
    draw.text((left_x + 62, top_y + 1), "Sentra", font=font_logo, fill=(255, 255, 255))

    # Version / Tag badge
    draw.rectangle([left_x + 225, top_y + 10, left_x + 360, top_y + 36], outline=(55, 55, 55), fill=(20, 20, 20), width=1)
    draw.text((left_x + 238, top_y + 13), "OCI RUNTIME", font=font_badge, fill=accent_rgb)

    # Domain in top right
    draw.text((width - 290, top_y + 12), "trysentra.tech", font=font_domain, fill=(160, 160, 160))

    # Main Headline
    head_y = 162
    draw.text((left_x, head_y), "Containers, minus the daemon.", font=font_title, fill=(255, 255, 255))

    # Subtitle
    sub_y = head_y + 80
    draw.text(
        (left_x, sub_y),
        "High-performance, daemonless OCI container runtime written in Go.",
        font=font_subtitle,
        fill=(195, 195, 195)
    )
    draw.text(
        (left_x, sub_y + 34),
        "Fast cached parallel builds, strict security defaults, and a tiny 8MB binary.",
        font=font_subtitle,
        fill=(140, 140, 140)
    )

    # Feature chips / pills (Zero radius)
    chips = ["DAEMONLESS", "8MB BINARY", "ROOTLESS", "SECURE SECCOMP", "OCI-NATIVE"]
    chip_x = left_x
    chip_y = sub_y + 88
    for chip in chips:
        bbox = draw.textbbox((0, 0), chip, font=font_badge)
        text_w = bbox[2] - bbox[0]
        box_w = text_w + 24
        draw.rectangle([chip_x, chip_y, chip_x + box_w, chip_y + 32], outline=(55, 55, 55), fill=(18, 18, 18), width=1)
        draw.text((chip_x + 12, chip_y + 7), chip, font=font_badge, fill=(225, 225, 225))
        chip_x += box_w + 12

    # Bottom Terminal Preview Box
    term_y = chip_y + 54
    term_w = width - (left_x * 2)
    term_h = 106
    # Outer terminal card
    draw.rectangle([left_x, term_y, left_x + term_w, term_y + term_h], outline=(50, 50, 50), fill=(13, 13, 13), width=1)
    
    # Terminal header
    draw.rectangle([left_x, term_y, left_x + term_w, term_y + 32], outline=(40, 40, 40), fill=(20, 20, 20), width=1)
    draw.ellipse([left_x + 16, term_y + 11, left_x + 26, term_y + 21], fill=(239, 68, 68))
    draw.ellipse([left_x + 34, term_y + 11, left_x + 44, term_y + 21], fill=(245, 158, 11))
    draw.ellipse([left_x + 52, term_y + 11, left_x + 62, term_y + 21], fill=accent_rgb)
    draw.text((left_x + 78, term_y + 7), "terminal -- sentra v0.1.0", font=font_term_meta, fill=(130, 130, 130))

    # Terminal command lines
    prompt_y = term_y + 44
    draw.text((left_x + 22, prompt_y), "$", font=font_term, fill=accent_rgb)
    draw.text((left_x + 42, prompt_y), "curl -fsSL https://trysentra.tech/install.sh | sh", font=font_term, fill=(255, 255, 255))
    
    prompt_y2 = prompt_y + 28
    draw.text((left_x + 22, prompt_y2), ">", font=font_term, fill=(100, 100, 100))
    draw.text((left_x + 42, prompt_y2), "Installed /usr/local/bin/sentra (8.29 MB, daemonless)", font=font_term, fill=(74, 222, 128))

    # Accent decorative bottom line
    draw.line([(0, height - 4), (width, height - 4)], fill=accent_rgb, width=4)

    # Save PNG
    output_path = os.path.join(public_dir, "og-image.png")
    img.save(output_path, "PNG", optimize=True)
    print(f"Generated {output_path} successfully ({os.path.getsize(output_path)} bytes)")

if __name__ == "__main__":
    generate_og_image()
