VERSION = 0.1.1

.PHONY: clean build push

push: build
	podman manifest push quay.io/barneygumble78/repliquay:$(VERSION)

build: clean
	podman build --platform linux/amd64,linux/arm64 --manifest quay.io/barneygumble78/repliquay:$(VERSION) .

clean:
	-podman manifest rm quay.io/barneygumble78/repliquay:$(VERSION)