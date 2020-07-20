all:
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o ./k8s-event .
	docker build -t aeronotix/k8s-event:$(shell cat .version) .

push:
	docker push aeronotix/k8s-event:$(shell cat .version)
