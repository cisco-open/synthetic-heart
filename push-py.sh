echo $1
export TAG=$1

echo "tagging bakshi41c/synheart..."
podman tag localhost/synheart-agent:dev-latest-py bakshi41c/synheart-agent:$TAG

echo "pushing bakshi41c/synheart..."
podman push bakshi41c/synheart-agent:$TAG

echo $1