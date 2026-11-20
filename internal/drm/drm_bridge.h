#ifndef ROUND_DRM_BRIDGE_H
#define ROUND_DRM_BRIDGE_H
#include <stddef.h>

typedef struct RoundDisplay RoundDisplay;
RoundDisplay *round_open(const char *device, const char *connector, char *error, size_t errorlen);
int round_present(RoundDisplay *d, const unsigned char *rgba, char *error, size_t errorlen);
void round_close(RoundDisplay *d);
#endif
