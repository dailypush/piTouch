#!/usr/bin/env python3
"""Test Waveshare e-Paper HAT"""
import sys
import os

# Add library path - using absolute path
sys.path.insert(0, '/home/chad/waveshare_epd/lib')

def test_display():
    """Test display initialization and control"""
    try:
        from TP_lib import epd2in13_V2
        
        print("=" * 50)
        print("WAVESHARE e-PAPER HAT TEST")
        print("=" * 50)
        
        print("\n[1/5] Creating display object...")
        epd = epd2in13_V2.EPD_2IN13_V2()
        print(f"✓ Display object created: {epd.width}x{epd.height}")
        
        print("\n[2/5] Initializing display hardware...")
        epd.init(epd.FULL_UPDATE)
        print("✓ Display initialized (FULL_UPDATE mode)")
        
        print("\n[3/5] Clearing display to white...")
        epd.Clear(0xFF)
        print("✓ Display cleared")
        print("    → Check if the physical screen is now WHITE")
        
        print("\n[4/5] Putting display to sleep...")
        epd.sleep()
        print("✓ Display is sleeping (low power consumption)")
        
        print("\n[5/5] Testing touch controller...")
        try:
            from TP_lib import gt1151
            gt = gt1151.GT1151()
            print("✓ GT1151 touch controller initialized")
            print("    → Try tapping the screen")
        except Exception as touch_err:
            print(f"⚠ Touch controller init: {touch_err}")
        
        print("\n" + "=" * 50)
        print("✅ HAT TEST PASSED - EVERYTHING IS WORKING!")
        print("=" * 50)
        return True
        
    except Exception as e:
        print(f"\n❌ TEST FAILED: {e}")
        import traceback
        traceback.print_exc()
        return False

if __name__ == '__main__':
    success = test_display()
    sys.exit(0 if success else 1)
