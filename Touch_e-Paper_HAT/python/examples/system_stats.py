#!/usr/bin/env python3
# -*- coding:utf-8 -*-
"""
System Stats Display for Waveshare Touch E-Paper HAT
Displays CPU, Memory, and Swap usage
Supports both real hardware and test/simulation mode
"""
import sys
import os
import time
import threading
import psutil
import logging
from PIL import Image, ImageDraw, ImageFont
import argparse

# Setup paths
script_dir = os.path.dirname(os.path.realpath(__file__))
parent_dir = os.path.dirname(script_dir)
libdir = os.path.join(parent_dir, 'lib')

if os.path.exists(libdir):
    sys.path.insert(0, libdir)

# Also check if Font.ttc is in the parent directory
fontdir = parent_dir

logging.basicConfig(level=logging.DEBUG)

# Initialize global objects
epd = None
touch = None
GT_Dev = None
GT_Old = None
ICNT_Dev = None
ICNT_Old = None
DISPLAY_TYPE = None
TEST_MODE = False

# Parse command line arguments
parser = argparse.ArgumentParser()
parser.add_argument('--test', action='store_true', help='Run in test/simulation mode without hardware')
parser.add_argument('--save-image', type=str, help='Save display output to image file (for testing)')
args = parser.parse_args()

if args.test:
    TEST_MODE = True
    DISPLAY_TYPE = "2.13"  # Default to 2.13" in test mode
    logging.info("Running in TEST MODE (no hardware required)")
else:
    # Import display drivers - we'll auto-detect which one is connected
    try:
        from TP_lib import epd2in13_V2
        from TP_lib import gt1151
        
        epd = epd2in13_V2.EPD_2IN13_V2()
        touch = gt1151.GT1151()
        GT_Dev = gt1151.GT_Development()
        GT_Old = gt1151.GT_Development()
        DISPLAY_TYPE = "2.13"
        logging.info("Using 2.13\" display with GT1151 touch controller")
    except Exception as e:
        try:
            from TP_lib import epd2in9_V2
            from TP_lib import icnt86
            
            epd = epd2in9_V2.EPD_2IN9_V2()
            touch = icnt86.INCT86()
            ICNT_Dev = icnt86.ICNT_Development()
            ICNT_Old = icnt86.ICNT_Development()
            DISPLAY_TYPE = "2.9"
            logging.info("Using 2.9\" display with ICNT86X touch controller")
        except Exception as e2:
            logging.error(f"Failed to initialize display: {e}")
            logging.info("Tip: Run with --test flag for simulation mode without hardware")
            sys.exit(1)

# Font setup - try to use Font.ttc if available, fall back to default
font_path = os.path.join(fontdir, 'Font.ttc')
try:
    font_large = ImageFont.truetype(font_path, 24)
    font_medium = ImageFont.truetype(font_path, 16)
    font_small = ImageFont.truetype(font_path, 12)
except Exception as font_error:
    logging.warning(f"Font.ttc not found at {font_path}, using default font")
    font_large = ImageFont.load_default()
    font_medium = ImageFont.load_default()
    font_small = ImageFont.load_default()

# Global flags
running = True
touch_detected = False
refresh_now = False

def touch_thread():
    """Monitor touch input"""
    global touch_detected, refresh_now
    
    logging.info("Touch thread started")
    while running:
        try:
            if DISPLAY_TYPE == "2.13":
                if touch.digital_read(touch.INT) == 0:
                    GT_Dev.Touch = 1
                else:
                    GT_Dev.Touch = 0
                    
                touch.GT_Scan(GT_Dev, GT_Old)
                if GT_Dev.TouchCount:
                    logging.info(f"Touch detected at X={GT_Dev.X[0]}, Y={GT_Dev.Y[0]}")
                    refresh_now = True
                    GT_Dev.TouchCount = 0
            else:
                if touch.digital_read(touch.INT) == 0:
                    ICNT_Dev.Touch = 1
                else:
                    ICNT_Dev.Touch = 0
                    
                touch.ICNT_Scan(ICNT_Dev, ICNT_Old)
                if ICNT_Dev.TouchCount:
                    logging.info(f"Touch detected at X={ICNT_Dev.X[0]}, Y={ICNT_Dev.Y[0]}")
                    refresh_now = True
                    ICNT_Dev.TouchCount = 0
        except Exception as e:
            logging.debug(f"Touch read error: {e}")
        
        time.sleep(0.05)
    
    logging.info("Touch thread exiting")

def get_system_stats():
    """Get current CPU, memory, and swap usage"""
    cpu_percent = psutil.cpu_percent(interval=0.1)
    
    mem = psutil.virtual_memory()
    mem_percent = mem.percent
    mem_used = mem.used // (1024 * 1024)  # Convert to MB
    mem_total = mem.total // (1024 * 1024)  # Convert to MB
    
    swap = psutil.swap_memory()
    swap_percent = swap.percent
    swap_used = swap.used // (1024 * 1024)  # Convert to MB
    swap_total = swap.total // (1024 * 1024)  # Convert to MB
    
    return {
        'cpu': cpu_percent,
        'mem_percent': mem_percent,
        'mem_used': mem_used,
        'mem_total': mem_total,
        'swap_percent': swap_percent,
        'swap_used': swap_used,
        'swap_total': swap_total,
        'timestamp': time.strftime("%H:%M:%S")
    }

def draw_bar(draw, x, y, width, height, value, color=0):
    """Draw a horizontal progress bar"""
    # Outline
    draw.rectangle([x, y, x + width, y + height], outline=color)
    # Filled portion
    fill_width = int(width * (value / 100))
    if fill_width > 0:
        draw.rectangle([x, y, x + fill_width, y + height], fill=color)

def draw_stats_screen(stats, display_width=None, display_height=None):
    """Create the stats display image"""
    # Get dimensions from epd if available, otherwise use parameters
    if display_width is None:
        display_width = epd.width if epd else 122
    if display_height is None:
        display_height = epd.height if epd else 250
    
    width = display_width
    height = display_height
    
    # Create white background image
    image = Image.new('L', (width, height), 255)
    draw = ImageDraw.Draw(image)
    
    # Title
    draw.text((2, 2), "System Stats", font=font_large, fill=0)
    
    # Timestamp
    draw.text((2, 28), f"Time: {stats['timestamp']}", font=font_small, fill=0)
    
    y_pos = 45
    line_height = 35
    
    # CPU
    draw.text((2, y_pos), f"CPU: {stats['cpu']:.1f}%", font=font_medium, fill=0)
    draw_bar(draw, 2, y_pos + 18, width - 5, 8, min(stats['cpu'], 100), color=0)
    
    # Memory
    y_pos += line_height
    draw.text((2, y_pos), f"Memory: {stats['mem_percent']:.1f}%", font=font_medium, fill=0)
    draw.text((2, y_pos + 14), f"  {stats['mem_used']}MB / {stats['mem_total']}MB", 
              font=font_small, fill=0)
    draw_bar(draw, 2, y_pos + 28, width - 5, 8, min(stats['mem_percent'], 100), color=0)
    
    # Swap
    y_pos += line_height
    draw.text((2, y_pos), f"Swap: {stats['swap_percent']:.1f}%", font=font_medium, fill=0)
    draw.text((2, y_pos + 14), f"  {stats['swap_used']}MB / {stats['swap_total']}MB", 
              font=font_small, fill=0)
    draw_bar(draw, 2, y_pos + 28, width - 5, 8, min(stats['swap_percent'], 100), color=0)
    
    # Footer
    footer_y = height - 15
    draw.text((2, footer_y), "Touch to refresh | Press Ctrl+C to exit", font=font_small, fill=0)
    
    return image

def main():
    """Main application loop"""
    global running, refresh_now, epd, touch, DISPLAY_TYPE
    
    # Set display dimensions based on type
    if DISPLAY_TYPE == "2.13":
        width, height = 122, 250
    else:  # 2.9"
        width, height = 128, 296
    
    try:
        if not TEST_MODE:
            logging.info("Initializing display...")
            try:
                if DISPLAY_TYPE == "2.13":
                    logging.debug("Calling epd.init(FULL_UPDATE)...")
                    epd.init(epd.FULL_UPDATE)
                    logging.debug("Calling touch.GT_Init()...")
                    touch.GT_Init()
                else:
                    logging.debug("Calling epd.init()...")
                    epd.init()
                    logging.debug("Calling touch.ICNT_Init()...")
                    touch.ICNT_Init()
                
                logging.debug("Clearing display...")
                epd.Clear(0xFF)
                logging.info("Display initialized successfully")
            except Exception as init_error:
                logging.error(f"Display initialization failed: {init_error}", exc_info=True)
                raise
        else:
            logging.info("Test mode: skipping hardware initialization")
        
        # Start touch thread only if not in test mode
        if not TEST_MODE:
            t = threading.Thread(target=touch_thread, daemon=True)
            t.start()
        
        update_count = 0
        last_update = 0
        
        logging.info("Starting main loop...")
        while running:
            try:
                current_time = time.time()
                
                # Update every 2 seconds or on touch
                if refresh_now or (current_time - last_update) > 2.0:
                    stats = get_system_stats()
                    image = draw_stats_screen(stats, width, height)
                    
                    logging.debug(f"CPU: {stats['cpu']:.1f}%, Mem: {stats['mem_percent']:.1f}%, Swap: {stats['swap_percent']:.1f}%")
                    
                    if TEST_MODE:
                        # In test mode, save image and print stats
                        if args.save_image:
                            image.save(args.save_image)
                            logging.info(f"Image saved to {args.save_image}")
                        print(f"\n=== System Stats (Updated: {stats['timestamp']}) ===")
                        print(f"CPU:    {stats['cpu']:6.1f}%  ", end="")
                        bar_len = int((stats['cpu'] / 100) * 20)
                        print("[" + "█" * bar_len + "░" * (20 - bar_len) + "]")
                        print(f"Memory: {stats['mem_percent']:6.1f}%  ", end="")
                        bar_len = int((stats['mem_percent'] / 100) * 20)
                        print("[" + "█" * bar_len + "░" * (20 - bar_len) + f"] {stats['mem_used']}MB / {stats['mem_total']}MB")
                        print(f"Swap:   {stats['swap_percent']:6.1f}%  ", end="")
                        bar_len = int((stats['swap_percent'] / 100) * 20)
                        print("[" + "█" * bar_len + "░" * (20 - bar_len) + f"] {stats['swap_used']}MB / {stats['swap_total']}MB")
                    else:
                        if DISPLAY_TYPE == "2.13":
                            if update_count == 0:
                                # Full update on first display
                                logging.info(f"Calling displayPartBaseImage for first update (update_count={update_count})")
                                epd.displayPartBaseImage(epd.getbuffer(image))
                                epd.init(epd.PART_UPDATE)
                            else:
                                # Partial update afterwards - use _Wait variant to ensure display completes
                                logging.info(f"Calling displayPartial_Wait (update_count={update_count})")
                                epd.displayPartial_Wait(epd.getbuffer(image))
                        else:
                            logging.info(f"Calling display_Partial_Wait for 2.9\" display")
                            epd.display_Partial_Wait(epd.getbuffer(image))
                    
                    update_count += 1
                    last_update = current_time
                    refresh_now = False
                
                time.sleep(0.1)
            
            except Exception as e:
                logging.error(f"Error in main loop: {e}")
                time.sleep(1)
    
    except KeyboardInterrupt:
        logging.info("Interrupt received")
    except Exception as e:
        logging.error(f"Fatal error: {e}", exc_info=True)
    finally:
        running = False
        logging.info("Cleaning up...")
        try:
            if epd is not None:
                logging.debug("Putting display to sleep...")
                epd.sleep()
        except Exception as cleanup_error:
            logging.debug(f"Error during cleanup: {cleanup_error}")
        
        logging.info("Done")

if __name__ == '__main__':
    main()
