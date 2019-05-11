.ONESHELL:

.PHONY: all
all: sync test lint sec

.PHONY: lint
lint:
	@docker-compose run --rm lint --pkg actor-system || :

.PHONY: sec
sec:
	@docker-compose run --rm sec --pkg actor-system || :

.PHONY: sync
sync:
	@docker-compose run --rm sync --pkg actor-system

.PHONY: test
test:
	@docker-compose run --rm test --pkg actor-system
