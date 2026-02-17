#!/usr/bin/env python3
import os
import sys
import time
from datetime import datetime

from PIL import Image, ImageDraw, ImageFont

BASE_DIR = "/home/chad/Touch_e-Paper_Code/python"
LIB_DIR = os.path.join(BASE_DIR, "lib")
FONT_PATH = os.path.join(BASE_DIR, "pic", "Font.ttc")

if LIB_DIR not in sys.path:
    sys.path.insert(0, LIB_DIR)

from TP_lib import epd2in13_V4, gt1151  # noqa: E402


BTN_THEME = (4, 2, 52, 20)
BTN_PAUSE = (56, 2, 104, 20)
BTN_REFRESH = (108, 2, 166, 20)
BTN_FUN = (170, 2, 246, 20)


def cpu_usage_sample(prev):
    with open("/proc/stat", "r", encoding="utf-8") as f:
        parts = f.readline().split()
    vals = list(map(int, parts[1:11]))
    user, nice, system, idle, iowait, irq, softirq, steal = vals[:8]

    idle_all = idle + iowait
    non_idle = user + nice + system + irq + softirq + steal
    total = idle_all + non_idle

    if prev is None:
        return 0.0, (total, idle_all)

    prev_total, prev_idle = prev
    total_diff = total - prev_total
    idle_diff = idle_all - prev_idle
    if total_diff <= 0:
        return 0.0, (total, idle_all)

    cpu = (total_diff - idle_diff) * 100.0 / total_diff
    return max(0.0, min(100.0, cpu)), (total, idle_all)


def mem_usage_sample():
    mem_total_kb = 0
    mem_avail_kb = 0
    with open("/proc/meminfo", "r", encoding="utf-8") as f:
        for line in f:
            if line.startswith("MemTotal:"):
                mem_total_kb = int(line.split()[1])
            elif line.startswith("MemAvailable:"):
                mem_avail_kb = int(line.split()[1])
    total_mb = mem_total_kb // 1024
    avail_mb = mem_avail_kb // 1024
    used_mb = total_mb - avail_mb
    if total_mb <= 0:
        return 0.0, 0, 0
    pct = used_mb * 100.0 / total_mb
    return pct, used_mb, total_mb


def draw_bar(draw, x, y, w, h, pct):
    draw.rectangle((x, y, x + w, y + h), outline=0, fill=255)
    fill_w = int(max(0.0, min(100.0, pct)) * w / 100.0)
    if fill_w > 0:
        draw.rectangle((x + 1, y + 1, x + fill_w - 1, y + h - 1), fill=0)


def draw_button(draw, rect, text, active=False):
    x0, y0, x1, y1 = rect
    fill = 0 if active else 255
    ink = 255 if active else 0
    draw.rounded_rectangle((x0, y0, x1, y1), radius=3, outline=0, fill=fill)
    tw = len(text) * 7
    tx = x0 + max(2, ((x1 - x0 + 1) - tw) // 2)
    ty = y0 + 4
    draw.text((tx, ty), text, fill=ink)


def draw_gauge(draw, cx, cy, r, pct, label):
    draw.ellipse((cx - r, cy - r, cx + r, cy + r), outline=0, fill=255)
    draw.arc((cx - r + 2, cy - r + 2, cx + r - 2, cy + r - 2), start=135, end=45, fill=0, width=2)
    # pointer
    angle = 135 - (270.0 * max(0.0, min(100.0, pct)) / 100.0)
    rad = angle * 3.14159265 / 180.0
    px = int(cx + (r - 6) * __import__("math").cos(rad))
    py = int(cy - (r - 6) * __import__("math").sin(rad))
    draw.line((cx, cy, px, py), fill=0, width=2)
    draw.ellipse((cx - 2, cy - 2, cx + 2, cy + 2), fill=0)
    draw.text((cx - r + 4, cy + r + 2), label, fill=0)


def draw_sparkline(draw, values, x, y, w, h):
    draw.rectangle((x, y, x + w, y + h), outline=0, fill=255)
    if len(values) < 2:
        return
    n = len(values)
    for i in range(1, n):
        x0 = x + int((i - 1) * (w - 2) / (n - 1)) + 1
        x1 = x + int(i * (w - 2) / (n - 1)) + 1
        y0 = y + h - 2 - int(max(0.0, min(100.0, values[i - 1])) * (h - 3) / 100.0)
        y1 = y + h - 2 - int(max(0.0, min(100.0, values[i])) * (h - 3) / 100.0)
        draw.line((x0, y0, x1, y1), fill=0, width=1)


def make_landscape_frame(
    epd,
    font_small,
    font_big,
    cpu_pct,
    mem_pct,
    mem_used,
    mem_total,
    clock_text,
    cpu_hist,
    paused,
    fun_mode,
    theme_mode,
    touch_marks,
):
    width = epd.height   # 250
    height = epd.width   # 122

    image = Image.new("1", (width, height), 255)
    draw = ImageDraw.Draw(image)

    # Layout guides
    draw.rectangle((0, 0, width - 1, height - 1), outline=0, fill=255)
    draw.line((0, 22, width - 1, 22), fill=0)
    draw.line((125, 23, 125, height - 1), fill=0)

    # Header
    draw.text((6, 4), "Pi Touch Dashboard", font=font_big, fill=0)
    draw.text((170, 5), clock_text, font=font_small, fill=0)
    draw_button(draw, BTN_THEME, "THEME", active=theme_mode)
    draw_button(draw, BTN_PAUSE, "PAUSE", active=paused)
    draw_button(draw, BTN_REFRESH, "REFRESH", active=False)
    draw_button(draw, BTN_FUN, "FUN", active=fun_mode)

    # Left side CPU panel
    draw.text((8, 28), f"CPU {cpu_pct:4.1f}%", font=font_small, fill=0)
    draw_bar(draw, 8, 46, 108, 12, cpu_pct)
    draw_sparkline(draw, cpu_hist, 8, 64, 108, 46)

    # Right side MEM panel
    draw.text((134, 28), f"MEM {mem_pct:4.1f}%", font=font_small, fill=0)
    draw.text((134, 46), f"{mem_used}MB / {mem_total}MB", font=font_small, fill=0)
    draw_bar(draw, 134, 64, 108, 12, mem_pct)

    # Fun gauges
    draw_gauge(draw, 160, 98, 16, cpu_pct, "CPU")
    draw_gauge(draw, 215, 98, 16, mem_pct, "MEM")

    if theme_mode:
        for xx in range(126, 250, 8):
            draw.line((xx, 24, xx, 120), fill=0)

    if fun_mode:
        for (mx, my, ttl) in touch_marks:
            if ttl <= 0:
                continue
            r = 2 + min(4, ttl)
            draw.ellipse((mx - r, my - r, mx + r, my + r), outline=0, fill=255)
            draw.line((mx - r - 2, my, mx + r + 2, my), fill=0)
            draw.line((mx, my - r - 2, mx, my + r + 2), fill=0)

    if paused:
        draw.rectangle((88, 46, 162, 76), outline=0, fill=255)
        draw.text((97, 56), "PAUSED", font=font_big, fill=0)

    return image


def region_partial_update_landscape(epd, full_buf, rect):
    # Rect is in landscape coordinates: width=250, height=122.
    lw = epd.height
    lh = epd.width
    x0, y0, x1, y1 = rect

    x0 = max(0, min(lw - 1, x0))
    x1 = max(0, min(lw - 1, x1))
    y0 = max(0, min(lh - 1, y0))
    y1 = max(0, min(lh - 1, y1))
    if x0 > x1 or y0 > y1:
        return

    # For landscape mode getbuffer() rotates image by 270 degrees.
    # Mapping to device coordinates: dx = (lh - 1 - y), dy = x
    dx0 = (lh - 1) - y1
    dx1 = (lh - 1) - y0
    dy0 = x0
    dy1 = x1

    dx0 = dx0 & ~0x7
    dx1 = dx1 | 0x7
    if dx1 > epd.width - 1:
        dx1 = epd.width - 1

    line_width = (epd.width + 7) // 8
    byte_start = dx0 >> 3
    byte_end = dx1 >> 3
    bytes_per_line = byte_end - byte_start + 1

    region = bytearray()
    for row in range(dy0, dy1 + 1):
        base = row * line_width
        region.extend(full_buf[base + byte_start: base + byte_start + bytes_per_line])

    epd2in13_V4.epdconfig.digital_write(epd.reset_pin, 0)
    epd2in13_V4.epdconfig.delay_ms(1)
    epd2in13_V4.epdconfig.digital_write(epd.reset_pin, 1)

    epd.send_command(0x01)
    epd.send_data(0xF9)
    epd.send_data(0x00)
    epd.send_data(0x00)

    epd.send_command(0x3C)
    epd.send_data(0x80)

    epd.send_command(0x11)
    epd.send_data(0x03)

    epd.SetWindow(dx0, dy0, dx1, dy1)
    epd.SetCursor(dx0 >> 3, dy0)
    epd.send_command(0x24)
    epd.send_data2(region)
    epd.TurnOnDisplayPart_Wait()


def main():
    epd = epd2in13_V4.EPD()
    gt = gt1151.GT1151()
    gt_dev = gt1151.GT_Development()
    gt_old = gt1151.GT_Development()
    font_small = ImageFont.truetype(FONT_PATH, 14)
    font_big = ImageFont.truetype(FONT_PATH, 18)

    print("Init full update")
    epd.init(epd.FULL_UPDATE)
    gt.GT_Init()
    epd.Clear(0xFF)

    prev_cpu = None
    last_cpu = None
    last_mem = None
    last_minute = None
    last_draw_ts = 0.0
    update_num = 0
    cpu_hist = []
    touch_marks = []
    paused = False
    fun_mode = True
    theme_mode = False
    force_full = False
    last_touch_xy = None
    last_touch_ts = 0.0

    full_every = 0
    min_draw_interval_s = 12
    cpu_threshold = 5.0
    mem_threshold = 2.0

    try:
        while True:
            if not paused:
                cpu_pct, prev_cpu = cpu_usage_sample(prev_cpu)
                mem_pct, mem_used, mem_total = mem_usage_sample()
            else:
                if last_cpu is None:
                    cpu_pct, prev_cpu = cpu_usage_sample(prev_cpu)
                    mem_pct, mem_used, mem_total = mem_usage_sample()
                else:
                    cpu_pct = last_cpu
                    mem_pct = last_mem
            # Touch poll
            if gt.digital_read(gt.INT) == 0:
                gt_dev.Touch = 1
            gt.GT_Scan(gt_dev, gt_old)

            update_num += 1
            minute_key = datetime.now().strftime("%H:%M")
            now = time.time()

            cpu_hist.append(cpu_pct)
            if len(cpu_hist) > 32:
                cpu_hist.pop(0)

            changed = []
            if gt_dev.TouchpointFlag:
                gt_dev.TouchpointFlag = 0
                tx = int(gt_dev.X[0])
                ty = int(gt_dev.Y[0])
                # Ignore known noisy defaults and out-of-range samples.
                if (tx == 0 and ty == 0) or tx > 121 or ty > 249:
                    tx = -1
                # Debounce repeated touch sample spam.
                if tx >= 0:
                    if last_touch_xy == (tx, ty) and (now - last_touch_ts) < 0.7:
                        tx = -1
                    else:
                        last_touch_xy = (tx, ty)
                        last_touch_ts = now
                if tx < 0:
                    pass
                else:
                # Map portrait touch coordinates to landscape canvas coordinates.
                    lx = max(0, min(epd.height - 1, ty))
                    ly = max(0, min(epd.width - 1, (epd.width - 1) - tx))

                    if BTN_THEME[0] <= lx <= BTN_THEME[2] and BTN_THEME[1] <= ly <= BTN_THEME[3]:
                        theme_mode = not theme_mode
                        changed.append((126, 23, 249, 121))
                        changed.append((0, 0, 124, 22))
                        print("TOUCH: theme toggle")
                    elif BTN_PAUSE[0] <= lx <= BTN_PAUSE[2] and BTN_PAUSE[1] <= ly <= BTN_PAUSE[3]:
                        paused = not paused
                        changed.append((0, 0, 124, 22))
                        changed.append((0, 23, 249, 121))
                        print(f"TOUCH: paused={paused}")
                    elif BTN_REFRESH[0] <= lx <= BTN_REFRESH[2] and BTN_REFRESH[1] <= ly <= BTN_REFRESH[3]:
                        force_full = True
                        changed.append((0, 0, 249, 121))
                        print("TOUCH: force full refresh")
                    elif BTN_FUN[0] <= lx <= BTN_FUN[2] and BTN_FUN[1] <= ly <= BTN_FUN[3]:
                        fun_mode = not fun_mode
                        changed.append((126, 23, 249, 121))
                        changed.append((125, 0, 249, 22))
                        print(f"TOUCH: fun={fun_mode}")
                    elif fun_mode:
                        touch_marks.append((lx, ly, 6))
                        changed.append((max(0, lx - 10), max(23, ly - 10), min(249, lx + 10), min(121, ly + 10)))
                        print(f"TOUCH: mark at ({lx},{ly})")

            # Decay touch marks and refresh only the right panel when active.
            new_marks = []
            marks_changed = False
            for mx, my, ttl in touch_marks:
                if ttl > 1:
                    new_marks.append((mx, my, ttl - 1))
                    marks_changed = True
            touch_marks = new_marks
            if marks_changed and fun_mode:
                changed.append((126, 23, 249, 121))

            if update_num == 1:
                changed.append((0, 0, epd.height - 1, epd.width - 1))
            else:
                if last_cpu is None or abs(cpu_pct - last_cpu) >= cpu_threshold:
                    changed.append((0, 23, 124, 121))
                if last_mem is None or abs(mem_pct - last_mem) >= mem_threshold:
                    changed.append((126, 23, 249, 121))
                if minute_key != last_minute:
                    changed.append((160, 0, 249, 22))

            if not changed or ((now - last_draw_ts) < min_draw_interval_s and not force_full):
                print(
                    f"SKIP {update_num}: cpu={cpu_pct:.1f}% mem={mem_pct:.1f}%"
                )
                time.sleep(1.5)
                continue

            frame = make_landscape_frame(
                epd,
                font_small,
                font_big,
                cpu_pct,
                mem_pct,
                mem_used,
                mem_total,
                minute_key,
                cpu_hist,
                paused,
                fun_mode,
                theme_mode,
                touch_marks,
            )
            buf = epd.getbuffer(frame)

            if force_full:
                epd.init(epd.FULL_UPDATE)
                epd.display(buf)
                epd.init(epd.PART_UPDATE)
                force_full = False
                print(f"FULL(touch) {update_num}: cpu={cpu_pct:.1f}% mem={mem_pct:.1f}%")
            elif update_num == 1:
                epd.displayPartBaseImage(buf)
                epd.init(epd.PART_UPDATE)
                print(f"FULL(base) {update_num}: cpu={cpu_pct:.1f}% mem={mem_pct:.1f}%")
            elif full_every > 0 and update_num % full_every == 0:
                epd.init(epd.FULL_UPDATE)
                epd.display(buf)
                epd.init(epd.PART_UPDATE)
                print(f"FULL {update_num}: cpu={cpu_pct:.1f}% mem={mem_pct:.1f}%")
            else:
                x0 = min(r[0] for r in changed)
                y0 = min(r[1] for r in changed)
                x1 = max(r[2] for r in changed)
                y1 = max(r[3] for r in changed)
                region_partial_update_landscape(epd, buf, (x0, y0, x1, y1))
                print(f"PART-REGION {update_num}: rect=({x0},{y0})-({x1},{y1})")

            last_cpu = cpu_pct
            last_mem = mem_pct
            last_minute = minute_key
            last_draw_ts = now
            time.sleep(1.5)

    except KeyboardInterrupt:
        print("Stopping")
    finally:
        epd.sleep()
        epd.Dev_exit()


if __name__ == "__main__":
    main()
