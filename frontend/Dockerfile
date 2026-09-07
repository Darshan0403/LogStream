# Stage 1: Build
FROM node:20-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
# Build same-origin: the frontend talks to nginx, which proxies /api and /ws to
# the Go server and injects the API key. No URLs or secrets are baked in.
RUN npm run build

# Stage 2: Serve
FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
# Rendered to /etc/nginx/conf.d/default.conf at container start with envsubst,
# substituting ${LOGSTREAM_API_KEY} from the environment.
COPY default.conf.template /etc/nginx/templates/default.conf.template
EXPOSE 80
