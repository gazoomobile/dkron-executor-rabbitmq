TAG=gazoo/dkron-executor-rabbitmq

.PHONY: build
build:
	docker build -t $(TAG) . && docker push $(TAG)