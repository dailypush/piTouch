#!/usr/bin/env python3
"""Reset and wake up the e-Paper display"""
import sys
import os
import time

sys.path.insert(0, '/home/chad/waveshare_epd/lib')

from TP_lib import epd2in13_V2
from PIL import Image, ImageDraw

try:
    print("Creating display object...")
    epd = epd2in13_V2.EPD_2IN13_V2()
    
    print("Waking up display...")
    epd.init(epd.FULL_UPDATE)
    
    print("Creating a clear white image...")
    image = Image.new('L', (epd.width, epd.height), 255)  # White background
    draw = ImageDraw.Draw(image)
    
    # Draw some text to show it's working
    draw.text((10, 10), "Display Reset", fill=0)
    draw.text((10, 30), "Ready for stats...", fill=0)
    
    print("Displaying test image...")
    epd.displayPartBaseImage(epd.getbuffer(image))
    epd.init(epd.PART_UPDATE)
    
    time.sleep(2)
    
    print("Clearing display...")
    epd.Clear(0xFF)
    
    print("Putting display to sleep...")
    epd.sleep()
    
    print("✓ Display reset complete!")
    
except Exception as e:
    print(f"✗ Error: {e}")
    import traceback
    traceback.print_exc()
