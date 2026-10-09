// crypto_diff.c: the native std/crypto (xo_crypto.c) on inputs from
// standard input, for internal/llvm's TestCryptoDiff, which compares its
// output with Go's crypto packages, std/rt's verify_password, and the
// RFC 9106 and x/crypto Argon2 vectors.
//
// Input, one operation per line, byte strings in hex ("-" is empty):
//   sha256 <data>
//   hmac <key> <msg>
//   pbkdf2 <password> <salt> <iterations> <key length>
//   argon2 <mode 0 d, 1 i, 2 id> <password> <salt> <secret> <data> <t> <m> <p> <key length>
//   b64 <url 0 or 1> <data>
//   verify <password> <hash>
//   equal <a> <b>
// Output, one line per operation: the result in hex, or 0 or 1.

#define XO_RT_NO_MAIN
#include "../xo_rt.c"

static uint8_t *unhex(const char *s, int64_t *n) {
	if (strcmp(s, "-") == 0) {
		*n = 0;
		return malloc(1);
	}
	int64_t l = (int64_t)strlen(s) / 2;
	uint8_t *p = malloc((size_t)l + 1);
	for (int64_t i = 0; i < l; i++) {
		p[i] = (uint8_t)(n_hex_val(s[2 * i]) << 4 | n_hex_val(s[2 * i + 1]));
	}
	*n = l;
	return p;
}

static void puthex(const uint8_t *p, int64_t n) {
	if (n == 0) {
		fputs("-", stdout);
	}
	for (int64_t i = 0; i < n; i++) {
		printf("%02x", p[i]);
	}
	putchar('\n');
}

int main(void) {
	static char line[1 << 20];
	char *f[12];
	while (fgets(line, sizeof line, stdin)) {
		int nf = 0;
		for (char *t = strtok(line, " \n"); t && nf < 12; t = strtok(NULL, " \n")) {
			f[nf++] = t;
		}
		if (nf == 0) {
			continue;
		}
		int64_t an, bn, cn, dn, len;
		if (strcmp(f[0], "sha256") == 0 && nf == 2) {
			uint8_t *a = unhex(f[1], &an);
			xo_rstr *r = xo_crypto_sha256((const char *)a, an, &len);
			puthex((uint8_t *)r->data, len);
			xo_free(r);
			free(a);
		} else if (strcmp(f[0], "hmac") == 0 && nf == 3) {
			uint8_t *a = unhex(f[1], &an), *b = unhex(f[2], &bn);
			xo_rstr *r = xo_crypto_hmac_sha256((const char *)a, an, (const char *)b, bn, &len);
			puthex((uint8_t *)r->data, len);
			xo_free(r);
			free(a);
			free(b);
		} else if (strcmp(f[0], "pbkdf2") == 0 && nf == 5) {
			uint8_t *a = unhex(f[1], &an), *b = unhex(f[2], &bn);
			xo_rstr *r = xo_crypto_pbkdf2((const char *)a, an, (const char *)b, bn, atoll(f[3]), atoll(f[4]), &len);
			puthex((uint8_t *)r->data, len);
			xo_free(r);
			free(a);
			free(b);
		} else if (strcmp(f[0], "argon2") == 0 && nf == 10) {
			uint8_t *a = unhex(f[2], &an), *b = unhex(f[3], &bn), *c = unhex(f[4], &cn), *d = unhex(f[5], &dn);
			uint32_t kl = (uint32_t)atoll(f[9]);
			uint8_t *out = malloc(kl);
			cr_argon2(atoi(f[1]), a, an, b, bn, c, cn, d, dn, (uint32_t)atoll(f[6]), (uint32_t)atoll(f[7]), (uint32_t)atoll(f[8]), out, kl);
			puthex(out, kl);
			free(out);
			free(a);
			free(b);
			free(c);
			free(d);
		} else if (strcmp(f[0], "b64") == 0 && nf == 3) {
			uint8_t *a = unhex(f[2], &an);
			xo_rstr *r = xo_crypto_b64((const char *)a, an, atoi(f[1]), &len);
			puthex((uint8_t *)r->data, len);
			xo_free(r);
			free(a);
		} else if (strcmp(f[0], "verify") == 0 && nf == 3) {
			uint8_t *a = unhex(f[1], &an), *b = unhex(f[2], &bn);
			printf("%lld\n", (long long)xo_crypto_verify_password((const char *)a, an, (const char *)b, bn));
			free(a);
			free(b);
		} else if (strcmp(f[0], "equal") == 0 && nf == 3) {
			uint8_t *a = unhex(f[1], &an), *b = unhex(f[2], &bn);
			printf("%lld\n", (long long)xo_crypto_equal((const char *)a, an, (const char *)b, bn));
			free(a);
			free(b);
		} else {
			printf("bad line\n");
		}
		fflush(stdout);
	}
	return 0;
}
