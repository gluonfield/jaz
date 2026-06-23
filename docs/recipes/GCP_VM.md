# GCP VM Backend

Recipe for a disposable Jaz backend VM on GCP.

## Create

```sh
PROJECT=jaz-dev
REGION=us-central1
ZONE=us-central1-a
NAME=jaz-v0046
RELEASE=v0.0.46
PORT=5299
TAG=jaz-backend
ADDRESS_NAME="$NAME-ip"

gcloud config set project "$PROJECT"
gcloud services enable compute.googleapis.com

gcloud compute addresses create "$ADDRESS_NAME" --region "$REGION"
IP="$(gcloud compute addresses describe "$ADDRESS_NAME" --region "$REGION" --format='value(address)')"

gcloud compute firewall-rules create allow-jaz-backend-$PORT \
  --allow "tcp:$PORT" \
  --source-ranges 0.0.0.0/0 \
  --target-tags "$TAG"

gcloud compute instances create "$NAME" \
  --zone "$ZONE" \
  --machine-type e2-medium \
  --image-family ubuntu-2404-lts-amd64 \
  --image-project ubuntu-os-cloud \
  --boot-disk-size 30GB \
  --tags "$TAG" \
  --address "$IP"
```

## Deploy Jaz

Use the reusable Linux setup in [`../remote-backend.md`](../remote-backend.md).
For this GCP VM, use:

```sh
gcloud compute ssh "$NAME" --zone "$ZONE"
```

Inside the VM, run the remote-backend server setup with the recipe release,
`JAZ_ADDR=:5299`, and `JAZ_PUBLIC_URL=http://<reserved-ip>:5299`. That generic
setup owns Node/npm, release install, systemd, restart, and boot startup.

## Verify

```sh
curl -fsS "http://$IP:$PORT/health"

gcloud compute ssh "$NAME" --zone "$ZONE" --command \
  "sudo systemctl show jaz -p Restart -p UnitFileState -p ExecStart --no-pager && sudo cat /var/lib/jaz/client-url.txt"
```

Expected health contains `"ok":true`; systemd should show `Restart=on-failure`
and `UnitFileState=enabled`.

## Destroy

```sh
gcloud compute instances delete "$NAME" --zone "$ZONE" --quiet
gcloud compute addresses delete "$ADDRESS_NAME" --region "$REGION" --quiet
gcloud compute firewall-rules delete allow-jaz-backend-$PORT --quiet
```
