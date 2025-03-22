clean:
	rm sensor-metrics

build: clean
	go build

docker-build-x86: build
	docker build -f Dockerfile.x86 -t ${DOCKER_HOST}/sensor-metrics:${BUILD_VERSION} .

docker-push-x86:
	docker push ${DOCKER_HOST}/sensor-metrics:${BUILD_VERSION}

k8s-deploy: 
