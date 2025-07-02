VERSION = 0.1.2

.PHONY: image clean build push

push: image
	podman manifest push quay.io/barneygumble78/repliquay:$(VERSION)

image: clean
	podman build --platform linux/amd64,linux/arm64 --manifest quay.io/barneygumble78/repliquay:$(VERSION) .

clean:
	-podman manifest rm quay.io/barneygumble78/repliquay:$(VERSION)
	-rm repliquay

build:
	go build -o repliquay