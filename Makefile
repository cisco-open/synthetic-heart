default: docker-all

docker-agent:
	@echo "Building agent container image"
	cd agent && podman build -f Dockerfile-NoPlugins -t synheart-agent:dev-latest-no-plugins ..
	cd agent && podman build -f Dockerfile -t synheart-agent:dev-latest ..

docker-agent-py:
	@echo "Building python agent container image"
	cd agent && podman build -f Dockerfile-Test-Py -t synheart-agent:dev-latest-py ..

.PHONY: docker-restapi
docker-restapi:
	@echo "Building restapi container image"
	cd restapi && podman build -f Dockerfile -t synheart-restapi:dev-latest ..

## Controller
.PHONY: docker-controller
docker-controller:
	@echo "Building controller container image"
	cd controller && podman build -f Dockerfile -t synheart-controller:dev-latest ..

.PHONY : docker-all
docker-all: clean docker-agent docker-restapi docker-controller

.PHONY : clean
clean:
	rm -rf agent/bin
	rm -rf controller/bin
	rm -rf restapi/bin