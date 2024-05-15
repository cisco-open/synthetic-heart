export TAG=$(date +%s)
podman tag localhost/synheart-controller:dev_latest bakshi41c/synheart-controller:$TAG
podman tag localhost/synheart-restapi:dev_latest bakshi41c/synheart-restapi:$TAG
podman tag localhost/synheart-agent:dev_latest bakshi41c/synheart-agent:$TAG
podman push bakshi41c/synheart-controller:$TAG
podman push bakshi41c/synheart-restapi:$TAG
podman push bakshi41c/synheart-agent:$TAG
echo $TAG