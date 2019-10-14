NAME:=logreport
VERSION:=0.0.1
MAINTAINER:="Masafumi Yamamoto <masa23@gmail.com>"
DESCRIPTION:="logreport is summarize LTSV logs and report to graphite"

clean:
	rm -rf build
	rm -f *.deb
	rm -f *.rpm

package-file:
	install -m 0755 -d build/root/usr/bin
	install -m 0755 -d build/root/etc
	install -m 0644 cmd/$(NAME)/config.sample.yaml build/root/etc/$(NAME).yaml 

build-arch:
	install -m 0700 -d build/
	cd cmd/$(NAME)
	GOOS=linux GOARCH=$(ARCH) go build -o build/$(NAME)_linux_$(ARCH)
	rm -f build/root/usr/bin/$(NAME)

create-deb:
	fpm -s dir -t deb -n $(NAME) -v $(VERSION) \
		--deb-priority optional --category admin \
		--package $(NAME)_$(VERSION)_$(ARCH).deb \
		--force \
		--description $(DESCRIPTION) \
		-m $(MAINTAINER) \
		--license "MIT" \
		-a $(ARCH) \
		build/root/=/

create-rpm:
	fpm -s dir -t rpm -n $(NAME) -v $(VERSION) \
		--package $(NAME)_$(VERSION)_$(FILE_ARCH).rpm \
		--force \
		--rpm-os linux \
		--description $(DESCRIPTION) \
		-m $(MAINTAINER) \
		--license "MIT" \
		-a $(ARCH) \
		--config-files /etc/ \
		build/root/=/ \
		build/$(NAME)_linux_$(ARCH)=/usr/bin/$(NAME)


build:
	make build-arch ARCH=amd64
	make build-arch ARCH=386
	make build-arch ARCH=arm
	make build-arch ARCH=arm64

deb:
	make build
	make package-file 
	make create-deb ARCH=amd64
	make create-deb ARCH=386
	make create-deb ARCH=arm
	make create-deb ARCH=arm64

rpm:
	make package-file 
	make create-rpm ARCH=amd64 FILE_ARCH=x86_64
	make create-rpm ARCH=386 FILE_ARCH=i686
	make create-rpm ARCH=arm FILE_ARCH=aarch32
	make create-rpm ARCH=arm64 FILE_ARCH=aarch64
