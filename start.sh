#!/bin/bash
# NOFXi startup script
cd "$(dirname "$0")"

# Load env
export RSA_PRIVATE_KEY=$(grep "^RSA_PRIVATE_KEY=" .env | sed 's/^RSA_PRIVATE_KEY=//' | sed 's/\\n/\n/g')
export DATA_ENCRYPTION_KEY=$(grep "^DATA_ENCRYPTION_KEY=" .env | sed 's/^DATA_ENCRYPTION_KEY=//')
export JWT_SECRET=$(grep "^JWT_SECRET=" .env | sed 's/^JWT_SECRET=//')

# Kill old processes
pkill -f "./nofx" 2>/dev/null
pkill -f "vite" 2>/dev/null
sleep 1

# Build
echo "🔨 Building NOFXi..."
go build -o nofx . || exit 1

# Start backend
echo "🚀 Starting backend (:8080)..."
./nofx > nofxi.log 2>&1 &
sleep 3

# Start frontend
echo "🎨 Starting frontend (:3000)..."
cd web && npx vite > /tmp/nofxi-web.log 2>&1 &
sleep 2

# Verify
echo ""
curl -s http://localhost:8080/api/agent/health > /dev/null && echo "✅ Backend: http://localhost:8080" || echo "❌ Backend failed"
curl -s http://localhost:3000/ > /dev/null && echo "✅ Frontend: http://localhost:3000" || echo "❌ Frontend failed"
echo ""
echo "NOFXi is ready 🧠"
