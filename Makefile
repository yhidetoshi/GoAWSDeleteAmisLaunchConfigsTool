GOCMD=go
GOBUILD=$(GOCMD) build
GOGET=$(GOCMD) get
CURRENT := $(shell pwd)
OUTPUTFILENAME := "main"
BUILDDIR=./build
DISTDIR=$(BUILDDIR)/dist
PKGDIR=$(BUILDDIR)/pkg
LDFLAGS := -X 'main.version=$(VERSION)'
GOXARCH := "386 amd64"
## GOXOUTPUT := "$(PKGDIR)/$(OUTPUTFILENAME)_{{.OS}}_{{.Arch}}/$(OUTPUTFILENAME)"
GOXOUTPUT := "$(PKGDIR)/$(NAME)_{{.OS}}_{{.Arch}}/{{.Dir}}"
VERSION := $(shell git describe --tags --abbrev=0)

.PHONY: setup
## Install dependencies
setup:
	$(GOGET) github.com/mitchellh/gox
	$(GOGET) github.com/tcnksm/ghr
	$(GOGET) -d -t ./...

.PHONY: cross-build
## Cross build binaries
cross-build:
	rm -rf $(PKGDIR)
	gox -os=$(GOXOS) -arch=$(GOXARCH) -output=$(GOXOUTPUT)

.PHONY: package
## Make package
package: cross-build
	rm -rf $(DISTDIR)
	mkdir $(DISTDIR)
	pushd $(PKGDIR) > /dev/null && \
		for P in `ls | xargs basename`; do zip -r $(CURRENT)/$(DISTDIR)/$$P.zip $$P; done && \
		popd > /dev/null

.PHONY: release
## Release package to Github
release: package
	ghr -u yhidetoshi -r GoAWSDeleteAmisLaunchConfigsTool $(VERSION) $(DISTDIR)
