
.PHONY: all
all:
	hack/build.sh

.PHONY: clean
clean:
	rm -rf bin/ _output/ go .version-defs

.PHONY: build
build:
	hack/build.sh

# Run test
.PHONY: test
test:
	hack/test-go.sh

.PHONY: verify
verify:
	hack/verify-all.sh

.PHONY: img
img:
	hack/build-img.sh

format:
	hack/format.sh

#  vim: set ts=2 sw=2 tw=0 noet :
