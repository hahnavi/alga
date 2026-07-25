#!/usr/bin/env bash
set -euo pipefail

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[alga]${NC} $*"; }
warn()  { echo -e "${YELLOW}[alga]${NC} $*"; }

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

generate_secret() {
    openssl rand -base64 32 | tr -d '\n'
}

info "Alga setup — generating configuration..."

if [ ! -f .env ]; then
    cp .env.example .env
    POSTGRES_PASS=$(generate_secret)
    VALKEY_PASS=$(generate_secret)
    RABBITMQ_PASS=$(generate_secret)
    sed -i "s|^POSTGRES_PASS=.*|POSTGRES_PASS=${POSTGRES_PASS}|" .env
    sed -i "s|^VALKEY_PASSWORD=.*|VALKEY_PASSWORD=${VALKEY_PASS}|" .env
    sed -i "s|^RABBITMQ_PASS=.*|RABBITMQ_PASS=${RABBITMQ_PASS}|" .env
    info "Generated root .env with random infrastructure passwords"
else
    warn "Root .env already exists, skipping"
fi

if [ ! -f apps/backend/.env ]; then
    cp apps/backend/.env.example apps/backend/.env
    ENCRYPTION_KEYS="1:$(generate_secret)"
    SECRET_PEPPER=$(generate_secret)
    cat >> apps/backend/.env <<EOF

ENCRYPTION_KEYS=${ENCRYPTION_KEYS}
SECRET_PEPPER=${SECRET_PEPPER}
POSTGRES_AUTO_MIGRATE=true
EOF
    info "Generated backend .env with encryption keys"
else
    warn "Backend .env already exists, skipping"
fi

if [ ! -f apps/frontend/.env ]; then
    touch apps/frontend/.env
    info "Created frontend .env"
fi

echo ""
info "Setup complete! Run the following to start Alga:"
echo ""
echo "    docker compose up -d"
echo ""
info "Then open http://localhost:3000"
info "First run: complete the setup wizard to create your initial admin account"
echo ""
