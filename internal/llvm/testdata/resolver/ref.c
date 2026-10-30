// The glibc side of TestResolverDiff (internal/llvm resolver_linux_test.go,
// decision 0206 stage 3): main.xo's program on the C library's
// getaddrinfo, printing what main.xo prints. XO_LLRT_C=net, which built
// main.xo on glibc's resolver, was deleted with the C runtime, so this is
// the reference: it listens on 127.0.0.1 and [::1] on one port, dials each
// argument (NAME, or NAME@PORT for a real resolution that prints "ok")
// and prints which listener answered or the error as Xo's net.dial
// words it (the port replaced by PORT).
#include <arpa/inet.h>
#include <errno.h>
#include <netdb.h>
#include <netinet/in.h>
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <unistd.h>

static void replace(char *s, const char *from, const char *to) {
	char out[1024];
	size_t n = strlen(from), o = 0;
	for (char *p = s; *p && o < sizeof out - 16;) {
		if (n > 0 && strncmp(p, from, n) == 0) {
			o += snprintf(out + o, sizeof out - o, "%s", to);
			p += n;
		} else {
			out[o++] = *p++;
		}
	}
	out[o] = 0;
	strcpy(s, out);
}

static int listen_on(int family, const char *ip, int port) {
	int fd = socket(family, SOCK_STREAM, 0);
	if (fd < 0) {
		return -1;
	}
	int one = 1;
	setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &one, sizeof one);
	if (family == AF_INET) {
		struct sockaddr_in a = {0};
		a.sin_family = AF_INET;
		a.sin_port = htons(port);
		inet_pton(AF_INET, ip, &a.sin_addr);
		if (bind(fd, (struct sockaddr *)&a, sizeof a) < 0) {
			return -1;
		}
	} else {
		struct sockaddr_in6 a = {0};
		a.sin6_family = AF_INET6;
		a.sin6_port = htons(port);
		inet_pton(AF_INET6, ip, &a.sin6_addr);
		if (bind(fd, (struct sockaddr *)&a, sizeof a) < 0) {
			return -1;
		}
	}
	listen(fd, 16);
	return fd;
}

struct ans {
	int fd;
	char tag;
};

static void *answer(void *p) {
	struct ans *a = p;
	for (;;) {
		int c = accept(a->fd, 0, 0);
		if (c < 0) {
			return 0;
		}
		write(c, &a->tag, 1);
		close(c);
	}
}

// dial_to connects to name:port as net.dial does: every address getaddrinfo
// gives, in order. It returns the socket, or -1 with the error in msg
// ("Refused" or "Io ...").
static int dial_to(const char *host, const char *port, const char *addr, char *msg, size_t n) {
	struct addrinfo hints = {0}, *res = 0;
	hints.ai_family = AF_UNSPEC;
	hints.ai_socktype = SOCK_STREAM;
	int g = getaddrinfo(*host ? host : "localhost", port, &hints, &res);
	if (g != 0) {
		snprintf(msg, n, "Io dial tcp %s: %s", addr, gai_strerror(g));
		return -1;
	}
	int e = 0;
	for (struct addrinfo *a = res; a; a = a->ai_next) {
		int fd = socket(a->ai_family, a->ai_socktype, 6);
		if (fd < 0) {
			e = errno;
			continue;
		}
		if (connect(fd, a->ai_addr, a->ai_addrlen) == 0) {
			freeaddrinfo(res);
			return fd;
		}
		e = errno;
		close(fd);
	}
	freeaddrinfo(res);
	if (e == ECONNREFUSED) {
		snprintf(msg, n, "Refused");
	} else {
		snprintf(msg, n, "Io dial tcp %s: connect: %s", addr, strerror(e));
	}
	return -1;
}

int main(int argc, char **argv) {
	int l4 = listen_on(AF_INET, "127.0.0.1", 0);
	struct sockaddr_in sa;
	socklen_t sl = sizeof sa;
	getsockname(l4, (struct sockaddr *)&sa, &sl);
	int port = ntohs(sa.sin_port);
	char ports[16];
	snprintf(ports, sizeof ports, "%d", port);
	int l6 = listen_on(AF_INET6, "::1", port);
	if (l6 < 0) {
		printf("listen [::1]: Io listen tcp [::1]:PORT: %s\n", strerror(errno));
		return 0;
	}
	struct ans a4 = {l4, '4'}, a6 = {l6, '6'};
	pthread_t t4, t6;
	pthread_create(&t4, 0, answer, &a4);
	pthread_create(&t6, 0, answer, &a6);
	for (int i = 1; i < argc; i++) {
		const char *name = argv[i];
		char msg[1024], addr[512], host[512], pp[32];
		const char *at = strchr(name, '@');
		int fd;
		if (at) {
			snprintf(host, sizeof host, "%.*s", (int)(at - name), name);
			snprintf(pp, sizeof pp, "%s", at + 1);
			snprintf(addr, sizeof addr, "%s:%s", host, pp);
			fd = dial_to(host, pp, addr, msg, sizeof msg);
			if (fd >= 0) {
				printf("%s: ok\n", name);
				close(fd);
			} else {
				replace(msg, pp, "PORT");
				printf("%s: %s\n", name, msg);
			}
			continue;
		}
		if (strchr(name, ':')) {
			snprintf(addr, sizeof addr, "[%s]:%s", name, ports);
		} else {
			snprintf(addr, sizeof addr, "%s:%s", name, ports);
		}
		fd = dial_to(name, ports, addr, msg, sizeof msg);
		if (fd < 0) {
			replace(msg, ports, "PORT");
			printf("%s: %s\n", name, msg);
			continue;
		}
		char b;
		ssize_t r = read(fd, &b, 1);
		if (r == 1) {
			printf("%s: %c\n", name, b);
		} else if (r == 0) {
			printf("%s: closed\n", name);
		} else {
			printf("%s: read Io read tcp: %s\n", name, strerror(errno));
		}
		close(fd);
	}
	fflush(stdout);
	return 0;
}
