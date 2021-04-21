.POSIX:
.SUFFIXES:

GO = go
RM = rm
SCDOC = scdoc
GOFLAGS =
PREFIX = /usr/local
BINDIR = $(PREFIX)/bin
MANDIR = $(PREFIX)/share/man
SYSCONFDIR = /etc
SHAREDSTATEDIR = /var/lib

all: soju sojuctl doc/soju.1

pkg = git.sr.ht/~emersion/soju
goflags = $(GOFLAGS) \
	-ldflags="-X '$(pkg)/config.sysConfDir=$(SYSCONFDIR)' \
		-X '$(pkg)/config.sharedStateDir=$(SHAREDSTATEDIR)'"

soju:
	$(GO) build $(goflags) ./cmd/soju
sojuctl:
	$(GO) build $(goflags) ./cmd/sojuctl
doc/soju.1: doc/soju.1.scd
	$(SCDOC) <doc/soju.1.scd >doc/soju.1

clean:
	$(RM) -rf soju sojuctl doc/soju.1
install: all
	mkdir -p $(DESTDIR)$(PREFIX)/$(BINDIR)
	mkdir -p $(DESTDIR)$(PREFIX)/$(MANDIR)/man1
	cp -f soju sojuctl $(DESTDIR)$(PREFIX)/$(BINDIR)
	cp -f doc/soju.1 $(DESTDIR)$(PREFIX)/$(MANDIR)/man1
