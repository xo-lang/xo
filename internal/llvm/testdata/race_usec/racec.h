// race_usec's C side: a function for many tasks to call.
#ifndef RACEC_H
#define RACEC_H

#include <stdint.h>

int64_t racec_add(int64_t a, int64_t b);

#endif
