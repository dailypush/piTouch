#include "Test.h"
#include "EPD_2in9_V2.h"
#include "ICNT86X.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <time.h>

extern ICNT86_Dev ICNT86_Dev_Now, ICNT86_Dev_Old;
extern int IIC_Address;
static pthread_t sysstat_t;
UBYTE sysstat_flag = 1;

UBYTE *SysStatImage;

void SysStats_Handler_2in9(int signo)
{
    printf("\r\nHandler:exit\r\n");
    EPD_2IN9_V2_Sleep();
    DEV_Delay_ms(2000);
    sysstat_flag = 0;
    pthread_join(sysstat_t, NULL);
    DEV_ModuleExit();
    exit(0);
}

void *sysstat_pthread_irq_2in9(void *arg)
{
    while(sysstat_flag) {
        if(DEV_Digital_Read(INT) == 0) {
            ICNT86_Dev_Now.Touch = 1;
        } else {
            ICNT86_Dev_Now.Touch = 0;
        }
        DEV_Delay_ms(10);
    }
    printf("thread:exit\r\n");
    pthread_exit(NULL);
}

// Get CPU usage from /proc/stat
double get_cpu_usage() {
    static unsigned long prev_idle = 0, prev_total = 0;
    unsigned long user, nice, system, idle;
    double cpu_percent = 0.0;
    
    FILE *fp = fopen("/proc/stat", "r");
    if (fp) {
        fscanf(fp, "cpu %lu %lu %lu %lu", &user, &nice, &system, &idle);
        fclose(fp);
        
        unsigned long curr_total = user + nice + system + idle;
        unsigned long curr_idle = idle;
        
        if (prev_total > 0) {
            unsigned long total_diff = curr_total - prev_total;
            unsigned long idle_diff = curr_idle - prev_idle;
            cpu_percent = (total_diff - idle_diff) * 100.0 / total_diff;
        }
        
        prev_idle = curr_idle;
        prev_total = curr_total;
    }
    return cpu_percent;
}

// Get memory usage from /proc/meminfo
void get_memory_usage(double *mem_percent, int *mem_used, int *mem_total) {
    unsigned long mem_total_kb = 0, mem_avail_kb = 0;
    
    FILE *fp = fopen("/proc/meminfo", "r");
    if (fp) {
        char line[256];
        while (fgets(line, sizeof(line), fp)) {
            if (strncmp(line, "MemTotal:", 9) == 0) {
                sscanf(line, "MemTotal: %lu", &mem_total_kb);
            } else if (strncmp(line, "MemAvailable:", 12) == 0) {
                sscanf(line, "MemAvailable: %lu", &mem_avail_kb);
            }
        }
        fclose(fp);
        
        *mem_total = mem_total_kb / 1024;
        int mem_free_mb = mem_avail_kb / 1024;
        *mem_used = *mem_total - mem_free_mb;
        *mem_percent = (*mem_used) * 100.0 / (*mem_total);
    }
}

void draw_bar_2in9(UBYTE *image, UWORD x, UWORD y, UWORD width, UWORD height, double percent) {
    UWORD bar_width = (UWORD)(width * percent / 100.0);
    if (bar_width > width) bar_width = width;
    
    // Draw filled bar
    for (UWORD yy = 0; yy < height; yy++) {
        for (UWORD xx = 0; xx < bar_width; xx++) {
            UWORD px = x + xx;
            UWORD py = y + yy;
            UWORD linewidth = (EPD_2IN9_V2_WIDTH % 8 == 0) ? (EPD_2IN9_V2_WIDTH / 8) : (EPD_2IN9_V2_WIDTH / 8 + 1);
            UWORD byte_idx = (px / 8) + py * linewidth;
            UBYTE bit_idx = 7 - (px % 8);
            
            if (byte_idx < linewidth * EPD_2IN9_V2_HEIGHT) {
                image[byte_idx] &= ~(1 << bit_idx);  // Set pixel to black
            }
        }
    }
    
    // Draw border
    for (UWORD xx = 0; xx < width + 2; xx++) {
        UWORD linewidth = (EPD_2IN9_V2_WIDTH % 8 == 0) ? (EPD_2IN9_V2_WIDTH / 8) : (EPD_2IN9_V2_WIDTH / 8 + 1);
        
        // Top border
        UWORD byte_idx = ((x - 1 + xx) / 8) + (y - 1) * linewidth;
        UBYTE bit_idx = 7 - ((x - 1 + xx) % 8);
        if (byte_idx < linewidth * EPD_2IN9_V2_HEIGHT) {
            image[byte_idx] &= ~(1 << bit_idx);
        }
        
        // Bottom border
        byte_idx = ((x - 1 + xx) / 8) + (y + height) * linewidth;
        bit_idx = 7 - ((x - 1 + xx) % 8);
        if (byte_idx < linewidth * EPD_2IN9_V2_HEIGHT) {
            image[byte_idx] &= ~(1 << bit_idx);
        }
    }
}

void update_stats_display_2in9() {
    static int update_count = 0;
    double cpu = get_cpu_usage();
    double mem_percent = 0;
    int mem_used = 0, mem_total = 0;
    get_memory_usage(&mem_percent, &mem_used, &mem_total);
    
    // Clear image (white)
    UWORD Imagesize = ((EPD_2IN9_V2_WIDTH % 8 == 0) ? (EPD_2IN9_V2_WIDTH / 8) : (EPD_2IN9_V2_WIDTH / 8 + 1)) * EPD_2IN9_V2_HEIGHT;
    for (UWORD i = 0; i < Imagesize; i++) {
        SysStatImage[i] = 0xFF;
    }
    
    // Draw text and bars
    Paint_SelectImage(SysStatImage);
    Paint_DrawString_EN(10, 10, "System Stats (2.9in)", &Font20, BLACK, WHITE);
    
    char buf[64];
    snprintf(buf, sizeof(buf), "CPU: %.1f%%", cpu);
    Paint_DrawString_EN(10, 40, buf, &Font16, BLACK, WHITE);
    draw_bar_2in9(SysStatImage, 10, 60, 200, 10, cpu);
    
    snprintf(buf, sizeof(buf), "Mem: %.1f%% (%dMB/%dMB)", mem_percent, mem_used, mem_total);
    Paint_DrawString_EN(10, 85, buf, &Font16, BLACK, WHITE);
    draw_bar_2in9(SysStatImage, 10, 105, 200, 10, mem_percent);
    
    snprintf(buf, sizeof(buf), "Update: %d", update_count++);
    Paint_DrawString_EN(10, 280, buf, &Font12, BLACK, WHITE);
    
    // Send to display
    if (update_count == 1) {
        EPD_2IN9_V2_Display_Base(SysStatImage);
        printf("*** First display update (base image) ***\r\n");
    } else {
        EPD_2IN9_V2_Display_Partial(SysStatImage);
        printf("*** Display update %d (cpu=%.1f%%, mem=%.1f%%) ***\r\n", update_count, cpu, mem_percent);
    }
}

int TestCode_SysStats_2in9(void)
{
    IIC_Address = 0x48;
    
    signal(SIGINT, SysStats_Handler_2in9);
    DEV_ModuleInit();
    
    pthread_create(&sysstat_t, NULL, sysstat_pthread_irq_2in9, NULL);
    
    EPD_2IN9_V2_Init();
    EPD_2IN9_V2_Clear();
    
    ICNT_Init();
    DEV_Delay_ms(100);
    
    // Create image buffer
    UWORD Imagesize = ((EPD_2IN9_V2_WIDTH % 8 == 0) ? (EPD_2IN9_V2_WIDTH / 8) : (EPD_2IN9_V2_WIDTH / 8 + 1)) * EPD_2IN9_V2_HEIGHT;
    if ((SysStatImage = (UBYTE *)malloc(Imagesize)) == NULL) {
        printf("Failed to allocate memory\r\n");
        return -1;
    }
    printf("Image buffer allocated: %d bytes\r\n", Imagesize);
    
    Paint_NewImage(SysStatImage, EPD_2IN9_V2_WIDTH, EPD_2IN9_V2_HEIGHT, 90, WHITE);
    Paint_SelectImage(SysStatImage);
    Paint_Clear(WHITE);
    
    Paint_DrawString_EN(30, 100, "System Stats", &Font24, BLACK, WHITE);
    Paint_DrawString_EN(30, 150, "2.9 inch Display", &Font20, BLACK, WHITE);
    Paint_DrawString_EN(30, 180, "Initializing...", &Font16, BLACK, WHITE);
    
    EPD_2IN9_V2_Display_Base(SysStatImage);
    
    printf("Starting main update loop...\r\n");
    int loop_count = 0;
    
    // Main loop - update every 2 seconds
    while (1) {
        loop_count++;
        update_stats_display_2in9();
        
        if (ICNT86_Dev_Now.Touch) {
            printf("Touch detected - forcing refresh\r\n");
            ICNT86_Dev_Now.Touch = 0;
        }
        
        sleep(2);  // Update every 2 seconds
    }
    
    return 0;
}
