import sys
import os
libdir = os.path.join(os.path.dirname(os.path.dirname(os.path.realpath(__file__))), 'lib')
if os.path.exists(libdir):
    sys.path.append(libdir)
sys.path.append('/home/pi/Touch_e-Paper_HAT/lib/TP_lib')

from PIL import Image, ImageDraw, ImageFont
from TP_lib import gt1151
from TP_lib import epd2in13_V3
import time
import random

# Initialize the e-Paper display
epd = epd2in13_V3.EPD()
epd.init(epd.FULL_UPDATE)
epd.Clear(0xFF)

# Drawing on the image
font15 = ImageFont.truetype(
    '/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf', 15)
image = Image.new('1', (epd.height, epd.width), 255)
draw = ImageDraw.Draw(image)

# Function to update the display with flight data


def update_display(flights):
    draw.rectangle((0, 0, epd.height, epd.width), fill=255)
    draw.text((10, 10), 'Tracked Flights:', font=font15, fill=0)
    y_offset = 30
    for flight in flights:
        flight_info = 'Flight: {} Altitude: {}ft'.format(
            flight.get('flight', 'N/A'), flight.get('altitude', 'N/A'))
        draw.text((10, y_offset), flight_info, font=font15, fill=0)
        y_offset += 20
    epd.display(epd.getbuffer(image))

# Generate random flight data for animation


def generate_random_flights():
    flight_count = random.randint(1, 4)
    flights = []
    for i in range(flight_count):
        flight_code = "FL" + str(random.randint(100, 999))
        altitude = random.randint(25000, 40000)
        flights.append({'flight': flight_code, 'altitude': altitude})
    return flights


# Main loop
try:
    while True:
        # Use randomly generated flight data for animation
        sample_flights = generate_random_flights()
        update_display(sample_flights)
        time.sleep(3)  # Update interval

except KeyboardInterrupt:
    print("Exiting...")
    epd.Clear(0xFF)
    epd.sleep()
    epd.Dev_exit()
    exit()
