#!/usr/bin/env python3
import sys
import os
import time

# Add library path
script_dir = os.path.dirname(os.path.realpath(__file__))
libdir = os.path.join(script_dir, 'lib')
sys.path.insert(0, libdir)

from TP_lib import epd2in13_V2
from PIL import Image, ImageDraw, ImageFont

print("Initializing display for FULL refresh test...")
epd = epd2in13_V2.EPD_2IN13_V2()

print("Init FULL_UPDATE mode...")
epd.init(epd.FULL_UPDATE)

print("Creating test image...")
image = Image.new('1', (epd.width, epd.height), 255)  # White background
draw = ImageDraw.Draw(image)

# Draw some test patterns
draw.rectangle((0, 0, epd.width-1, epd.height-1), outline=0)  # Border
draw.rectangle((10, 10, epd.width-10, epd.height-10), outline=0) 

# Draw text
font24 = ImageFont.load_default()
draw.text((20, 50), "FULL REFRESH TEST", font=font24, fill=0)
draw.text((20, 80), f"Time: {time.strftime('%H:%M:%S')}", font=font24, fill=0)

# Draw diagonal lines
for i in range(0, epd.width, 20):
    draw.line([(i, 0), (i, epd.height)], fill=0)

print("Sending image to display with FULL update...")
epd.display(epd.getbuffer(image))

print("Waiting 3 seconds...")
time.sleep(3)

print("Putting display to sleep...")
epd.sleep()

print("Test complete! Display should show test pattern.")
