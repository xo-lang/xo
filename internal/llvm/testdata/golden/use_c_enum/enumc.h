#ifndef ENUMC_H
#define ENUMC_H
enum enumc_mode { ENUMC_UP, ENUMC_DOWN };
enum enumc_level { ENUMC_LEVEL_LOW = 1, ENUMC_LEVEL_MID = 5, ENUMC_LEVEL_HIGH, ENUMC_LEVEL_MAX = ENUMC_LEVEL_HIGH };
int enumc_twice(int x);
#endif
