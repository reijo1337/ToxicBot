SHELL := /bin/bash

GO?=go
GOPATH?=$(shell go env GOPATH)
GOPACKAGES=$(shell go list ./...)
GOLANGCI_LINT_VERSION ?= v2.11.3
GOLANGCI_LINT_DOCKER = docker run --rm \
	-v $(CURDIR):/app \
	-w /app \
	golangci/golangci-lint:$(GOLANGCI_LINT_VERSION)

### билдит докер образ для выравнивания структур
align-build:
	DOCKER_SCAN_SUGGEST=false docker build -t golang-align-check --no-cache - < $(PWD)/tools/align.Dockerfile

### выравнивает структуры для меньшей аллокации
align:
	docker run --rm -v $(PWD):/app -w /app golang-align-check fieldalignment -fix ./...

### врубает линтер
lint:
	$(GOLANGCI_LINT_DOCKER) golangci-lint run ./...

### выравнивает импорты
imports:
	docker run --rm -v $(pwd):/data cytopia/goimports -d .

fmt:
	$(GOLANGCI_LINT_DOCKER) golangci-lint fmt ./...

migration:
	docker run -v $(PWD)/db/migrations:/migrations migrate/migrate:v4.18.3 create -ext sql -dir /migrations -seq $(name)

dozzlepwd:
	docker run --rm httpd:alpine htpasswd -bnBC 10 "" $(password) | tr -d ':\n'

### создаёт и пушит новый patch-тег (vX.Y.Z -> vX.Y.Z+1)
release-patch:
	@$(MAKE) --no-print-directory _release BUMP=patch

### создаёт и пушит новый minor-тег (vX.Y.Z -> vX.Y+1.0)
release-minor:
	@$(MAKE) --no-print-directory _release BUMP=minor

### создаёт и пушит новый major-тег (vX.Y.Z -> vX+1.0.0)
release-major:
	@$(MAKE) --no-print-directory _release BUMP=major

_release:
	@branch=$$(git rev-parse --abbrev-ref HEAD); \
	if [ "$$branch" != "master" ]; then \
		echo "Error: must be on master, currently on $$branch"; exit 1; \
	fi; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "Error: working tree is dirty, commit or stash changes first"; exit 1; \
	fi; \
	git fetch --quiet origin master; \
	if [ "$$(git rev-parse HEAD)" != "$$(git rev-parse origin/master)" ]; then \
		echo "Error: local master is not in sync with origin/master, pull/push first"; exit 1; \
	fi; \
	last=$$(git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null || echo v0.0.0); \
	read X Y Z <<< $$(echo $$last | awk -F '[v.]' '{print $$2, $$3, $$4}'); \
	case "$(BUMP)" in \
		patch) Z=$$((Z + 1));; \
		minor) Y=$$((Y + 1)); Z=0;; \
		major) X=$$((X + 1)); Y=0; Z=0;; \
		*) echo "Error: BUMP must be patch|minor|major, got '$(BUMP)'"; exit 1;; \
	esac; \
	new="v$$X.$$Y.$$Z"; \
	echo "Current: $$last -> New: $$new"; \
	read -p "Create tag $$new and push to origin? [y/N] " ans; \
	case "$$ans" in \
		y|Y) git tag $$new && git push origin $$new;; \
		*) echo "Aborted"; exit 0;; \
	esac