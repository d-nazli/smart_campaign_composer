# syntax=docker/dockerfile:1

# ---- dev: Vite dev server (docker-compose.dev.yml) and `make test` ----------
FROM node:22-alpine AS dev
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
EXPOSE 5173
CMD ["npm", "run", "dev"]

# ---- build: static bundle ---------------------------------------------------
FROM dev AS build
RUN npm run build

# ---- runtime (default target): nginx serves the SPA and proxies /api --------
FROM nginx:1.27-alpine AS runtime
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
