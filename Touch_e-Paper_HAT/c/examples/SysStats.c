#include "Test.h"
#include "EPD_2in13_V2.h"
#include "GT1151.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#define SYSSTATS_FORCE_FULL_REFRESH 1

extern GT1151_Dev Dev_Now, Dev_Old;
extern int IIC_Address;
static pthread_t sysstat_t;
UBYTE sysstat_flag = 1;

UBYTE *SysStatImage;

void SysStats_Handler(int signo)
{
    printf("\r\nHandler:exit\r\n");
    EPD_2IN13_V2_Sleep();
    DEV_Delay_ms(2000);
    sysstat_flag = 0;
    pthread_join(sysstat_t, NULL);
    DEV_ModuleExit();
    exit(0);
}

void *sysstat_pthread_irq(void *arg)
{
    while(sysstat_flag) {
        if(DEV_Digital_Read(INT) == 0) {
            Dev_Now.Touch = 1;
        } else {
            Dev_Now.Touch = 0;
        }
        usleep(200000);
    }
    printf("thread:exit\r\n");
    pthread_exit(NULL);
}

// Get CPU usage from /proc/stat
double get_cpu_usage(void) {
    static unsigned long long prev_idle = 0;
    static unsigned long long prev_total = 0;

    unsigned long long user = 0, nice = 0, system = 0, idle = 0;
    unsigned long long iowait = 0, irq = 0, softirq = 0, steal = 0;
    unsigned long long guest = 0, guest_nice = 0;

    FILE *fp = fopen("/proc/stat", "r");
    if (fp == NULL) {
        return 0.0;
    }

    int read = fscanf(
        fp,
        "cpu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu",
        &user, &nice, &system, &idle, &iowait,
        &irq, &softirq, &steal, &guest, &guest_nice
    );
    fclose(fp);
    if (read < 4) {
        return 0.0;
    }

    unsigned long long idle_all = idle + iowait;
    unsigned long long non_idle = user + nice + system + irq + softirq + steal;
    unsigned long long total = idle_all + non_idle;

    if (prev_total == 0) {
        prev_total = total;
        prev_idle = idle_all;
        return 0.0;
    }

    unsigned long long total_diff = total - prev_total;
    unsigned long long idle_diff = idle_all - prev_idle;

    prev_total = total;
    prev_idle = idle_all;

    if (total_diff == 0) {
        return 0.0;
    }

    return (double)(total_diff - idle_diff) * 100.0 / (double)total_diff;
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

void draw_bar(UBYTE *image, UWORD x, UWORD y, UWORD width, UWORD height, double percent) {
    UWORD bar_width = (UWORD)(width * percent / 100.0);
    if (bar_width > width) bar_width = width;
    
    // Draw filled bar
    for (UWORD yy = 0; yy < height; yy++) {
        for (UWORD xx = 0; xx < bar_width; xx++) {
            UWORD px = x + xx;
            UWORD py = y + yy;
            UWORD linewidth = (EPD_2IN13_V2_WIDTH % 8 == 0) ? (EPD_2IN13_V2_WIDTH / 8) : (EPD_2IN13_V2_WIDTH / 8 + 1);
            UWORD byte_idx = (px / 8) + py * linewidth;
            UBYTE bit_idx = 7 - (px % 8);
            
            if (byte_idx < linewidth * EPD_2IN13_V2_HEIGHT) {
                image[byte_idx] &= ~(1 << bit_idx);  // Set pixel to black
            }
        }
    }
    
    // Draw border
    for (UWORD xx = 0; xx < width + 2; xx++) {
        UWORD linewidth = (EPD_2IN13_V2_WIDTH % 8 == 0) ? (EPD_2IN13_V2_WIDTH / 8) : (EPD_2IN13_V2_WIDTH / 8 + 1);
        
        // Top border
        UWORD byte_idx = ((x - 1 + xx) / 8) + (y - 1) * linewidth;
        UBYTE bit_idx = 7 - ((x - 1 + xx) % 8);
        if (byte_idx < linewidth * EPD_2IN13_V2_HEIGHT) {
            image[byte_idx] &= ~(1 << bit_idx);
        }
        
        // Bottom border
        byte_idx = ((x - 1 + xx) / 8) + (y + height) * linewidth;
        bit_idx = 7 - ((x - 1 + xx) % 8);
        if (byte_idx < linewidth * EPD_2IN13_V2_HEIGHT) {
            image[byte_idx] &= ~(1 << bit_idx);
        }
    }
}

void update_stats_display() {
    static int update_count = 0;
    double cpu = get_cpu_usage();
    double mem_percent = 0;
    int mem_used = 0, mem_total = 0;
    get_memory_usage(&mem_percent, &mem_used, &mem_total);
    
    // Clear image (white)
    UWORD Imagesize = ((EPD_2IN13_V2_WIDTH % 8 == 0) ? (EPD_2IN13_V2_WIDTH / 8) : (EPD_2IN13_V2_WIDTH / 8 + 1)) * EPD_2IN13_V2_HEIGHT;
    for (UWORD i = 0; i < Imagesize; i++) {
        SysStatImage[i] = 0xFF;
    }
    
    // Draw text and bars
    Paint_SelectImage(SysStatImage);
    Paint_SetMirroring(MIRROR_HORIZONTAL);
    Paint_DrawString_EN(2, 2, "System Stats", &Font20, BLACK, WHITE);
    
    char buf[64];
    snprintf(buf, sizeof(buf), "CPU: %.1f%%", cpu);
    Paint_DrawString_EN(2, 30, buf, &Font16, BLACK, WHITE);
    draw_bar(SysStatImage, 2, 50, 100, 8, cpu);
    
    snprintf(buf, sizeof(buf), "Memory: %.1f%%", mem_percent);
    Paint_DrawString_EN(2, 70, buf, &Font16, BLACK, WHITE);
    snprintf(buf, sizeof(buf), "%dMB / %dMB", mem_used, mem_total);
    Paint_DrawString_EN(2, 85, buf, &Font12, BLACK, WHITE);
    draw_bar(SysStatImage, 2, 95, 100, 8, mem_percent);
    
    snprintf(buf, sizeof(buf), "Update: %d", update_count++);
    Paint_DrawString_EN(2, 230, buf, &Font12, BLACK, WHITE);
    
    if (SYSSTATS_FORCE_FULL_REFRESH || update_count == 1 || (update_count % 30) == 0) {
        EPD_2IN13_V2_Init(EPD_2IN13_V2_FULL);
        EPD_2IN13_V2_DisplayPartBaseImage(SysStatImage);
        EPD_2IN13_V2_Init(EPD_2IN13_V2_PART);
        printf("*** FULL update %d (cpu=%.1f%%, mem=%.1f%%) ***\r\n", update_count, cpu, mem_percent);
    } else {
        EPD_2IN13_V2_DisplayPart_Wait(SysStatImage);
        printf("*** PART update %d (cpu=%.1f%%, mem=%.1f%%) ***\r\n", update_count, cpu, mem_percent);
    }
}

int TestCode_SysStats(void)
{
    IIC_Address = 0x14;
    
    signal(SIGINT, SysStats_Handler);
    DEV_ModuleInit();
    
    pthread_create(&sysstat_t, NULL, sysstat_pthread_irq, NULL);
    
    EPD_2IN13_V2_Init(EPD_2IN13_V2_FULL);
    EPD_2IN13_V2_Clear();
    
    GT_Init();
    DEV_Delay_ms(100);
    
    // Create image buffer
    UWORD Imagesize = ((EPD_2IN13_V2_WIDTH % 8 == 0) ? (EPD_2IN13_V2_WIDTH / 8) : (EPD_2IN13_V2_WIDTH / 8 + 1)) * EPD_2IN13_V2_HEIGHT;
    if ((SysStatImage = (UBYTE *)malloc(Imagesize)) == NULL) {
        printf("Failed to allocate memory\r\n");
        return -1;
    }
    printf("Image buffer allocated: %d bytes\r\n", Imagesize);
    
    Paint_NewImage(SysStatImage, EPD_2IN13_V2_WIDTH, EPD_2IN13_V2_HEIGHT, 0, WHITE);
    Paint_SelectImage(SysStatImage);
    Paint_SetMirroring(MIRROR_HORIZONTAL);
    Paint_Clear(WHITE);
    
    Paint_DrawString_EN(5, 50, "System Stats Monitor", &Font24, BLACK, WHITE);
    Paint_DrawString_EN(5, 100, "Starting display...", &Font16, BLACK, WHITE);
    
    EPD_2IN13_V2_DisplayPartBaseImage(SysStatImage);
    EPD_2IN13_V2_Init(EPD_2IN13_V2_PART);
    
    printf("Starting main update loop...\r\n");
    int loop_count = 0;
    
    // Main loop - update every 2 seconds
    while (1) {
        loop_count++;
        update_stats_display();
        
        if (Dev_Now.Touch) {
            printf("Touch detected - forcing refresh\r\n");
            Dev_Now.Touch = 0;
        }
        
        sleep(5);  // Update every 5 seconds to reduce self-load
    }
    
    return 0;
}
