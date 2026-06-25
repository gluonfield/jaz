# GCP VM Backend

Recipe for a disposable Jaz backend VM on GCP.

## Create

```sh
PROJECT=jaz-dev
REGION=us-central1
ZONE=us-central1-a
NAME=jaz-backend
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

If that fails with `Permission denied (publickey)`, an old SHA-1 RSA key is
likely being offered, which Ubuntu 24.04's sshd rejects. Add a fresh ed25519
key to the instance metadata and connect with it directly:

```sh
ssh-keygen -t ed25519 -f /tmp/jaz_ed25519 -N "" -C "$USER" -q
gcloud compute instances add-metadata "$NAME" --zone "$ZONE" \
  --metadata ssh-keys="$USER:$(cat /tmp/jaz_ed25519.pub)"
ssh -i /tmp/jaz_ed25519 -o StrictHostKeyChecking=no "$USER@$IP"
```

Inside the VM, run the remote-backend server setup (it installs the latest
release) with `JAZ_ADDR=:5299` and `JAZ_PUBLIC_URL=http://<reserved-ip>:5299`.
That generic setup owns Node/npm, release install, systemd, restart, and boot
startup.
Install the agent CLIs you want to sign into (`npm install -g @openai/codex
@anthropic-ai/claude-code`); without them the onboarding agent cards show
"Not installed" and offer no sign-in.

This leaves a plain-HTTP backend at `http://<reserved-ip>:5299`, which the
desktop app connects to directly. A browser client (served over HTTPS) cannot
reach a plain-HTTP backend (mixed content); to use one, serve the app and backend
behind one HTTPS origin — see
[Self-host the app and backend behind one origin](../remote-backend.md).

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
