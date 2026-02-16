import time
import epd2in13_V3
from PIL import Image, ImageDraw


def create_plane_image():
    """Create an image of the plane from a string representation."""
    plane_repr = ("   X   ",
                  "X XXX X",
                  " XXXXX ",
                  "X XXX X",
                  "   X   ")

    img = Image.new('1', (len(plane_repr[0]), len(plane_repr)), 255)
    draw = ImageDraw.Draw(img)

    for y, row in enumerate(plane_repr):
        for x, pixel in enumerate(row):
            if pixel == "X":
                draw.point((x, y), fill=0)

    return img


def main():
    # Initialize the e-Paper display
    epd = epd2in13_V3.EPD()
    epd.init(epd.FULL_UPDATE)
    epd.Clear(0xFF)

    plane_image = create_plane_image()

    try:
        while True:
            for x_offset in range(0, epd.height + plane_image.width, 5):
                # Create a new blank image
                image = Image.new('1', (epd.height, epd.width), 255)
                draw = ImageDraw.Draw(image)

                # Paste the plane image at the current offset
                image.paste(plane_image, (epd.height - x_offset, 30))

                # Update the e-Paper display
                epd.display(epd.getbuffer(image))

                # Sleep between updates to control animation speed
                time.sleep(0.3)

    except KeyboardInterrupt:
        print("Exiting...")
        epd.Clear(0xFF)
        epd.sleep()
        epd.Dev_exit()
        exit()


if __name__ == "__main__":
    main()
