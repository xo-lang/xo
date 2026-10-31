package rt

// The status code constants of std/http (decision 0269): Http_OK and the
// rest, as values of the generated newtype Http_Status.
var (
	Http_CONTINUE                        = Http_Status{F0: 100}
	Http_SWITCHING_PROTOCOLS             = Http_Status{F0: 101}
	Http_PROCESSING                      = Http_Status{F0: 102}
	Http_EARLY_HINTS                     = Http_Status{F0: 103}
	Http_OK                              = Http_Status{F0: 200}
	Http_CREATED                         = Http_Status{F0: 201}
	Http_ACCEPTED                        = Http_Status{F0: 202}
	Http_NON_AUTHORITATIVE_INFO          = Http_Status{F0: 203}
	Http_NO_CONTENT                      = Http_Status{F0: 204}
	Http_RESET_CONTENT                   = Http_Status{F0: 205}
	Http_PARTIAL_CONTENT                 = Http_Status{F0: 206}
	Http_MULTI_STATUS                    = Http_Status{F0: 207}
	Http_ALREADY_REPORTED                = Http_Status{F0: 208}
	Http_IM_USED                         = Http_Status{F0: 226}
	Http_MULTIPLE_CHOICES                = Http_Status{F0: 300}
	Http_MOVED_PERMANENTLY               = Http_Status{F0: 301}
	Http_FOUND                           = Http_Status{F0: 302}
	Http_SEE_OTHER                       = Http_Status{F0: 303}
	Http_NOT_MODIFIED                    = Http_Status{F0: 304}
	Http_USE_PROXY                       = Http_Status{F0: 305}
	Http_TEMPORARY_REDIRECT              = Http_Status{F0: 307}
	Http_PERMANENT_REDIRECT              = Http_Status{F0: 308}
	Http_BAD_REQUEST                     = Http_Status{F0: 400}
	Http_UNAUTHORIZED                    = Http_Status{F0: 401}
	Http_PAYMENT_REQUIRED                = Http_Status{F0: 402}
	Http_FORBIDDEN                       = Http_Status{F0: 403}
	Http_NOT_FOUND                       = Http_Status{F0: 404}
	Http_METHOD_NOT_ALLOWED              = Http_Status{F0: 405}
	Http_NOT_ACCEPTABLE                  = Http_Status{F0: 406}
	Http_PROXY_AUTH_REQUIRED             = Http_Status{F0: 407}
	Http_REQUEST_TIMEOUT                 = Http_Status{F0: 408}
	Http_CONFLICT                        = Http_Status{F0: 409}
	Http_GONE                            = Http_Status{F0: 410}
	Http_LENGTH_REQUIRED                 = Http_Status{F0: 411}
	Http_PRECONDITION_FAILED             = Http_Status{F0: 412}
	Http_REQUEST_ENTITY_TOO_LARGE        = Http_Status{F0: 413}
	Http_REQUEST_URI_TOO_LONG            = Http_Status{F0: 414}
	Http_UNSUPPORTED_MEDIA_TYPE          = Http_Status{F0: 415}
	Http_REQUESTED_RANGE_NOT_SATISFIABLE = Http_Status{F0: 416}
	Http_EXPECTATION_FAILED              = Http_Status{F0: 417}
	Http_TEAPOT                          = Http_Status{F0: 418}
	Http_MISDIRECTED_REQUEST             = Http_Status{F0: 421}
	Http_UNPROCESSABLE_ENTITY            = Http_Status{F0: 422}
	Http_LOCKED                          = Http_Status{F0: 423}
	Http_FAILED_DEPENDENCY               = Http_Status{F0: 424}
	Http_TOO_EARLY                       = Http_Status{F0: 425}
	Http_UPGRADE_REQUIRED                = Http_Status{F0: 426}
	Http_PRECONDITION_REQUIRED           = Http_Status{F0: 428}
	Http_TOO_MANY_REQUESTS               = Http_Status{F0: 429}
	Http_REQUEST_HEADER_FIELDS_TOO_LARGE = Http_Status{F0: 431}
	Http_UNAVAILABLE_FOR_LEGAL_REASONS   = Http_Status{F0: 451}
	Http_INTERNAL_SERVER_ERROR           = Http_Status{F0: 500}
	Http_NOT_IMPLEMENTED                 = Http_Status{F0: 501}
	Http_BAD_GATEWAY                     = Http_Status{F0: 502}
	Http_SERVICE_UNAVAILABLE             = Http_Status{F0: 503}
	Http_GATEWAY_TIMEOUT                 = Http_Status{F0: 504}
	Http_HTTP_VERSION_NOT_SUPPORTED      = Http_Status{F0: 505}
	Http_VARIANT_ALSO_NEGOTIATES         = Http_Status{F0: 506}
	Http_INSUFFICIENT_STORAGE            = Http_Status{F0: 507}
	Http_LOOP_DETECTED                   = Http_Status{F0: 508}
	Http_NOT_EXTENDED                    = Http_Status{F0: 510}
	Http_NETWORK_AUTHENTICATION_REQUIRED = Http_Status{F0: 511}
)
