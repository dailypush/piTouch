#include "EPD_2in13_V3.h"
#include <stdlib.h>
#include <string.h>
#include <stdio.h>

extern int IIC_Address;

int main(void)
{
    IIC_Address = 0x14;
    if (DEV_ModuleInit() != 0) {
        printf("DEV_ModuleInit failed\n");
        return 1;
    }

    UWORD width = (EPD_2in13_V3_WIDTH % 8 == 0) ? (EPD_2in13_V3_WIDTH / 8) : (EPD_2in13_V3_WIDTH / 8 + 1);
    UWORD height = EPD_2in13_V3_HEIGHT;
    UWORD image_size = width * height;
    UBYTE *image = (UBYTE *)malloc(image_size);
    if (image == NULL) {
        printf("malloc failed\n");
        DEV_ModuleExit();
        return 1;
    }

    printf("V3 probe: init full\n");
    EPD_2in13_V3_Init(EPD_2IN13_V3_FULL);

    printf("V3 probe: black fill\n");
    memset(image, 0x00, image_size);
    EPD_2in13_V3_Display(image);
    DEV_Delay_ms(1500);

    printf("V3 probe: white fill\n");
    memset(image, 0xFF, image_size);
    EPD_2in13_V3_Display(image);
    DEV_Delay_ms(1500);

    printf("V3 probe: sleep\n");
    EPD_2in13_V3_Sleep();
    DEV_ModuleExit();
    free(image);
    return 0;
}
