VERSION = 0.1

.PHONY: clean

push: build
	podman manifest push quay.io/barneygumble78/repliquay:$(VERSION)

build: clean
	podman build --platform linux/amd64,linux/arm64 --manifest quay.io/barneygumble78/repliquay:$(VERSION) .

clean:
	-podman untag quay.io/barneygumble78/repliquay:$(VERSION)