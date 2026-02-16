#include "EPD_2in13_V3.h"
#include "ICNT86X.h"
#include "Paint.h"
#include "DEV_Config.h"
#include "GUI_BMPfile.h"
#include "time.h"

#define MAX_FLIGHTS 5

typedef struct
{
    char flight_number[10];
    char departure[20];
    char arrival[20];
    int altitude;
    int speed;
} FlightInfo;

// Mock flight data
FlightInfo flights[MAX_FLIGHTS] = {
    {"AA123", "New York", "Los Angeles", 30000, 500},
    {"DL456", "Chicago", "San Francisco", 32000, 520},
    {"UA789", "Houston", "Seattle", 31000, 510},
    {"BA101", "London", "New York", 33000, 530},
    {"LH202", "Berlin", "Paris", 29000, 490},
};

void display_flight(UBYTE *BlackImage, FlightInfo flight)
{
    Paint_Clear(WHITE);
    Paint_DrawString_EN(5, 5, flight.flight_number, &Font16, WHITE, BLACK);
    Paint_DrawString_EN(5, 30, flight.departure, &Font12, WHITE, BLACK);
    Paint_DrawString_EN(5, 50, flight.arrival, &Font12, WHITE, BLACK);

    char buffer[20];
    sprintf(buffer, "Altitude: %d ft", flight.altitude);
    Paint_DrawString_EN(5, 70, buffer, &Font12, WHITE, BLACK);

    sprintf(buffer, "Speed: %d knots", flight.speed);
    Paint_DrawString_EN(5, 90, buffer, &Font12, WHITE, BLACK);

    EPD_2in13_V3_Display_Base(BlackImage);
}

int main(void)
{
    UWORD Imagesize = ((EPD_2in13_V3_WIDTH % 8 == 0) ? (EPD_2in13_V3_WIDTH / 8) : (EPD_2in13_V3_WIDTH / 8 + 1)) * EPD_2in13_V3_HEIGHT;
    UBYTE *BlackImage = (UBYTE *)malloc(Imagesize);

    DEV_ModuleInit();
    EPD_2in13_V3_Init(EPD_2IN13_V3_FULL);
    EPD_2in13_V3_Clear();

    Paint_NewImage(BlackImage, EPD_2in13_V3_WIDTH, EPD_2in13_V3_HEIGHT, 0, WHITE);
    Paint_SelectImage(BlackImage);
    Paint_SetMirroring(MIRROR_ORIGIN);
    Paint_Clear(WHITE);

    int current_flight = 0;
    while (1)
    {
        display_flight(BlackImage, flights[current_flight]);
        current_flight = (current_flight + % MAX_FLIGHTS;
sleep(3); // Wait for 3 seconds before displaying the next flight
    }
    // Clean up and exit
    EPD_2in13_V3_Sleep();
    free(BlackImage);
    DEV_ModuleExit();
    return 0;
}
