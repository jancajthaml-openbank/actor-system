.ONESHELL:

.PHONY: all
all: sync test lint sec

.PHONY: fetch
fetch:
	@docker-compose run fetch

.PHONY: lint
lint:
	docker-compose run --rm lint

.PHONY: sec
sec:
	@docker-compose run --rm sec || :

.PHONY: sync
sync:
	@docker-compose run --rm sync

.PHONY: test
test:
	@docker-compose run --rm test
