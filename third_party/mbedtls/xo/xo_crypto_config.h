// xo_crypto_config.h: the Xo runtime's changes to TF-PSA-Crypto's default
// configuration (TF_PSA_CRYPTO_USER_CONFIG_FILE, decision 0105).

// Native programs run tasks on several threads: the PSA key store and
// the random generator are shared.
#define MBEDTLS_THREADING_C
#define MBEDTLS_THREADING_PTHREAD

// no files: keys are volatile, certificates and keys come from memory
#undef MBEDTLS_FS_IO
#undef MBEDTLS_PSA_CRYPTO_STORAGE_C
#undef MBEDTLS_PSA_ITS_FILE_C
#undef MBEDTLS_SELF_TEST

// what TLS 1.2 and 1.3 with ECDHE, ECDSA, RSA, AES-GCM, AES-CBC, and
// ChaCha20-Poly1305 never use
#undef MBEDTLS_LMS_C
#undef MBEDTLS_NIST_KW_C
#undef MBEDTLS_PKCS5_C
#undef MBEDTLS_PEM_WRITE_C
#undef PSA_WANT_ALG_CCM
#undef PSA_WANT_ALG_CCM_STAR_NO_TAG
#undef PSA_WANT_ALG_CMAC
#undef PSA_WANT_ALG_CFB
#undef PSA_WANT_ALG_OFB
#undef PSA_WANT_ALG_FFDH
#undef PSA_WANT_ALG_JPAKE
#undef PSA_WANT_ALG_MD5
#undef PSA_WANT_ALG_PBKDF2_HMAC
#undef PSA_WANT_ALG_PBKDF2_AES_CMAC_PRF_128
#undef PSA_WANT_ALG_RIPEMD160
#undef PSA_WANT_ALG_RSA_PKCS1V15_CRYPT
#undef PSA_WANT_ALG_SHA_224
#undef PSA_WANT_ALG_SHA3_224
#undef PSA_WANT_ALG_SHA3_256
#undef PSA_WANT_ALG_SHA3_384
#undef PSA_WANT_ALG_SHA3_512
#undef PSA_WANT_ALG_SHAKE128
#undef PSA_WANT_ALG_SHAKE256
#undef PSA_WANT_ALG_TLS12_PSK_TO_MS
#undef PSA_WANT_ALG_TLS12_ECJPAKE_TO_PMS
#undef PSA_WANT_ECC_BRAINPOOL_P_R1_256
#undef PSA_WANT_ECC_BRAINPOOL_P_R1_384
#undef PSA_WANT_ECC_BRAINPOOL_P_R1_512
#undef PSA_WANT_ECC_MONTGOMERY_448
#undef PSA_WANT_ECC_SECP_K1_256
#undef PSA_WANT_DH_RFC7919_2048
#undef PSA_WANT_DH_RFC7919_3072
#undef PSA_WANT_DH_RFC7919_4096
#undef PSA_WANT_DH_RFC7919_6144
#undef PSA_WANT_DH_RFC7919_8192
#undef PSA_WANT_KEY_TYPE_PASSWORD
#undef PSA_WANT_KEY_TYPE_PASSWORD_HASH
#undef PSA_WANT_KEY_TYPE_ARIA
#undef PSA_WANT_KEY_TYPE_CAMELLIA
#undef PSA_WANT_KEY_TYPE_DH_PUBLIC_KEY
#undef PSA_WANT_KEY_TYPE_DH_KEY_PAIR_BASIC
#undef PSA_WANT_KEY_TYPE_DH_KEY_PAIR_IMPORT
#undef PSA_WANT_KEY_TYPE_DH_KEY_PAIR_EXPORT
#undef PSA_WANT_KEY_TYPE_DH_KEY_PAIR_GENERATE
#undef PSA_WANT_KEY_TYPE_RSA_KEY_PAIR_GENERATE

// HACL* X25519 (Everest): the ECDHE of most handshakes, about twice as
// fast as the generic Montgomery ladder
#define MBEDTLS_ECDH_VARIANT_EVEREST_ENABLED
// the widest window of point multiplication (P-256 ECDSA: about 5 percent
// faster than the default 4, for 32 points of memory per operation)
#define MBEDTLS_ECP_WINDOW_SIZE 6

// SHA-256 and SHA-512 with the ARMv8 crypto extension when the CPU has it
// (checked once at run time, the portable C otherwise; nothing changes on
// x86-64, where the library has no SHA instructions path). AES already
// uses AES-NI and the ARMv8 extension (MBEDTLS_AESNI_C, MBEDTLS_AESCE_C
// are on by default), as do the precomputed tables of the curves'
// generators (MBEDTLS_ECP_FIXED_POINT_OPTIM). Decision 0117.
#define MBEDTLS_SHA256_USE_ARMV8_A_CRYPTO_IF_PRESENT
#define MBEDTLS_SHA512_USE_A64_CRYPTO_IF_PRESENT
