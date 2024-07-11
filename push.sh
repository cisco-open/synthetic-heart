echo $1
export TAG=$1

echo "tagging bakshi41c/synheart..."
podman tag localhost/synheart-controller:dev-latest bakshi41c/synheart-controller:$TAG
podman tag localhost/synheart-restapi:dev-latest bakshi41c/synheart-restapi:$TAG
podman tag localhost/synheart-agent:dev-latest bakshi41c/synheart-agent:$TAG
podman tag localhost/synheart-agent:dev-latest-no-plugins bakshi41c/synheart-agent:$TAG-no-plugins
podman tag localhost/synheart-agent:dev-latest-with-py bakshi41c/synheart-agent:$TAG-with-py

echo "tagging containers.cisco.com/synthetic-heart"
podman tag localhost/synheart-controller:dev-latest containers.cisco.com/synthetic-heart/controller:$TAG
podman tag localhost/synheart-restapi:dev-latest containers.cisco.com/synthetic-heart/restapi:$TAG
podman tag localhost/synheart-agent:dev-latest containers.cisco.com/synthetic-heart/agent:$TAG
podman tag localhost/synheart-agent:dev-latest-no-plugins containers.cisco.com/synthetic-heart/agent:$TAG-no-plugins
podman tag localhost/synheart-agent:dev-latest-with-py containers.cisco.com/synthetic-heart/agent:$TAG-with-py

echo "pushing bakshi41c/synheart..."
podman push bakshi41c/synheart-controller:$TAG
podman push bakshi41c/synheart-restapi:$TAG
podman push bakshi41c/synheart-agent:$TAG
podman push bakshi41c/synheart-agent:$TAG-no-plugins
podman push bakshi41c/synheart-agent:$TAG-with-py

echo "tagging containers.cisco.com/synthetic-heart..."
#podman push containers.cisco.com/synthetic-heart/controller:$TAG
#podman push containers.cisco.com/synthetic-heart/restapi:$TAG
#podman push containers.cisco.com/synthetic-heart/agent:$TAG
#podman push containers.cisco.com/synthetic-heart/agent:$TAG-no-plugins
#podman push containers.cisco.com/synthetic-heart/agent:$TAG-with-py

echo $1