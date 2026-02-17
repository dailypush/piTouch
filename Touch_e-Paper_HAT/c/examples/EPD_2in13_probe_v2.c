#include "EPD_2in13_V2.h"
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

    UWORD width = (EPD_2IN13_V2_WIDTH % 8 == 0) ? (EPD_2IN13_V2_WIDTH / 8) : (EPD_2IN13_V2_WIDTH / 8 + 1);
    UWORD height = EPD_2IN13_V2_HEIGHT;
    UWORD image_size = width * height;
    UBYTE *image = (UBYTE *)malloc(image_size);
    if (image == NULL) {
        printf("malloc failed\n");
        DEV_ModuleExit();
        return 1;
    }

    printf("V2 probe: init full\n");
    EPD_2IN13_V2_Init(EPD_2IN13_V2_FULL);

    printf("V2 probe: black fill\n");
    memset(image, 0x00, image_size);
    EPD_2IN13_V2_Display(image);
    DEV_Delay_ms(1500);

    printf("V2 probe: white fill\n");
    memset(image, 0xFF, image_size);
    EPD_2IN13_V2_Display(image);
    DEV_Delay_ms(1500);

    printf("V2 probe: sleep\n");
    EPD_2IN13_V2_Sleep();
    DEV_ModuleExit();
    free(image);
    return 0;
}
