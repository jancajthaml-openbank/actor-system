export COMPOSE_DOCKER_CLI_BUILD = 1
export DOCKER_BUILDKIT = 1
export COMPOSE_PROJECT_NAME = actor-system

.ONESHELL:

.PHONY: all
all: sync test lint sec

.PHONY: version
version:
	@\
		docker \
		compose \
		run \
		--rm version \
	|| :

.PHONY: lint
lint:
	@\
		docker \
		compose \
		run \
		--rm lint \
		--source /project \
	|| :

.PHONY: sec
sec:
	@\
		docker \
		compose \
		run \
		--rm sec \
		--source /project \
	|| :

.PHONY: sync
sync:
	@\
		docker \
		compose \
		run \
		--rm sync \
		--source /project

.PHONY: test
test:
	@\
		docker \
		compose \
		run \
		--rm test \
		--source /project \
		--output /project/reports/unit-tests
