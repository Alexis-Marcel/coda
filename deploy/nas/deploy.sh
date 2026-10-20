#!/bin/bash
# Met à jour la stack du NAS avec la dernière image publiée sur ghcr (à lancer une fois l'action « Image Docker » finie).
# Usage : deploy/nas/deploy.sh   (depuis la racine du dépôt ; NASA=hôte ssh, par défaut nasa)
set -euo pipefail
HOST=${NASA:-nasa}
PROJ=/boot/config/plugins/compose.manager/projects/coda
scp -q deploy/nas/compose.yaml deploy/nas/compose.override.yaml "$HOST:$PROJ/"
ssh "$HOST" "mkdir -p /mnt/user/appdata/coda/data && chown 65532:65532 /mnt/user/appdata/coda/data \
  && cd $PROJ && docker compose -f compose.yaml -f compose.override.yaml pull \
  && docker compose -f compose.yaml -f compose.override.yaml up -d && docker image prune -f >/dev/null"
