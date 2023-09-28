# SPDX-License-Identifier: BSD-3-Clause
#
# Authors: Alexander Jung <a.jung@lancs.ac.uk>
#
# Copyright (c) 2021, Lancaster University.  All rights reserved.
#
# Redistribution and use in source and binary forms, with or without
# modification, are permitted provided that the following conditions
# are met:
#
# 1. Redistributions of source code must retain the above copyright
#    notice, this list of conditions and the following disclaimer.
# 2. Redistributions in binary form must reproduce the above copyright
#    notice, this list of conditions and the following disclaimer in the
#    documentation and/or other materials provided with the distribution.
# 3. Neither the name of the copyright holder nor the names of its
#    contributors may be used to endorse or promote products derived from
#    this software without specific prior written permission.
#
# THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
# AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
# IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
# ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
# LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
# CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
# SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
# INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
# CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
# ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
# POSSIBILITY OF SUCH DAMAGE.

WORKDIR     ?= $(CURDIR)
DISTDIR     ?= $(WORKDIR)/dist
REG         ?= ghcr.io
ORG         ?= unikraft
REPO        ?= governance
BIN         ?= governctl

#
# Versioning
#
ifeq ($(HASH),)
HASH_COMMIT ?= HEAD
HASH        ?= $(shell git update-index -q --refresh && \
                       git describe --tags)
# Others can't be dirty by definition
ifneq ($(HASH_COMMIT),HEAD)
HASH_COMMIT ?= HEAD
endif
DIRTY       ?= $(shell git update-index -q --refresh && \
                       git diff-index --quiet HEAD -- $(WORKDIR) || \
                       echo "-dirty")
endif
APP_VERSION ?= $(HASH)$(DIRTY)
GIT_SHA     ?= $(shell git update-index -q --refresh && \
                       git rev-parse --short HEAD)

#
# Tools
#
DOCKER      ?= docker
DOCKER_RUN  ?= $(DOCKER) run --rm $(1) \
               -p $(PORT):$(PORT) \
               -w /go/src/github.com/$(ORG)/$(REPO) \
               -v $(WORKDIR):/go/src/github.com/$(ORG)/$(REPO) \
               $(REGISTRY)/$(ORG)/$(REPO):$(IMAGE_TAG) \
                 $(2)
GO          ?= go

# Misc
Q           ?= @

# If run with DOCKER= or within a container, unset DOCKER_RUN so all commands
# are not proxied via docker container.
ifeq ($(DOCKER),)
DOCKER_RUN  :=
else ifneq ($(wildcard /.dockerenv),)
DOCKER_RUN  :=
endif
.PROXY      :=
ifneq ($(DOCKER_RUN),)
.PROXY      := docker-proxy-
$(MAKECMDGOALS):
	$(info Running target via Docker ($(IMAGE)...))
	$(Q)$(call DOCKER_RUN,,$(MAKE) $@)
endif

#
# Targets
#
.PHONY: all
$(.PROXY)all: $(BIN)

ifeq ($(DEBUG),y)
$(.PROXY)$(BIN): GO_GCFLAGS ?= -N -l
endif
$(.PROXY)$(BIN): GO_LDFLAGS ?= -s -w
$(.PROXY)$(BIN): GO_LDFLAGS += -X "github.com/unikraft/governance/internal/version.version=$(APP_VERSION)"
$(.PROXY)$(BIN): GO_LDFLAGS += -X "github.com/unikraft/governance/internal/version.commit=$(GIT_SHA)"
$(.PROXY)$(BIN): GO_LDFLAGS += -X "github.com/unikraft/governance/internal/version.buildTime=$(shell date)"
$(.PROXY)$(BIN):
	$(GO) build \
		-ldflags='$(GO_GCFLAGS)' \
		-ldflags='$(GO_LDFLAGS)' \
		-o $(DISTDIR)/$@ \
		$(WORKDIR)/cmd/$@

.PHONY: container
container: TARGET ?= build
container: GOLANG_VERSION ?= 1.15
ifeq ($(TARGET),devenv)
container: TAG ?= devenv
else
container: TAG ?= latest
endif
container: IMAGE ?= $(REG)/$(ORG)/$(REPO):$(TAG)
container: 
	$(DOCKER) build \
		--build-arg GOLANG_VERSION=$(GOLANG_VERSION) \
		--tag $(IMAGE) \
		--file $(WORKDIR)/Dockerfile \
		$(WORKDIR)

.PHONY: devenv
devenv:
	$(DOCKER) run -it --rm \
		--name $(REPO)-devenv \
		-e GITHUB_ORG=$(ORG) \
		-v $(WORKDIR):/go/src/github.com/$(ORG)/$(REPO) \
		$(REG)/$(ORG)/$(REPO):devenv