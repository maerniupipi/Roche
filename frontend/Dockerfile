ARG NODE_BASE_IMAGE=node:22-alpine

# Build the legacy frontend mounted at `/`.
FROM ${NODE_BASE_IMAGE} AS frontend-builder

WORKDIR /app/frontend

ARG NPM_REGISTRY=https://registry.npmmirror.com
ARG COMMIT_ID_ARG

ENV VITE_IS_DOCKER=true
ENV VITE_FRONTEND_COMMIT=${COMMIT_ID_ARG}
ENV NODE_OPTIONS=--max-old-space-size=4096

COPY frontend/package.json ./
COPY frontend/packages/ ./packages/
RUN npm config set registry ${NPM_REGISTRY} && \
    npm install --no-package-lock

COPY frontend/ ./
RUN npm install --no-package-lock && npm run build

# Build the new frontend mounted at `/default/`.
FROM ${NODE_BASE_IMAGE} AS frontend-default-builder

WORKDIR /app/frontend-default

ARG NPM_REGISTRY=https://registry.npmmirror.com
ARG COMMIT_ID_ARG

ENV VITE_IS_DOCKER=true
ENV VITE_FRONTEND_COMMIT=${COMMIT_ID_ARG}
ENV NODE_OPTIONS=--max-old-space-size=4096

COPY frontend-default/package.json ./
COPY frontend-default/packages/ ./packages/
RUN npm config set registry ${NPM_REGISTRY} && \
    npm install --no-package-lock

COPY frontend-default/ ./
RUN npm install --no-package-lock && npm run build

# Build the admin frontend mounted at `/admin/`.
FROM ${NODE_BASE_IMAGE} AS frontend-admin-builder

WORKDIR /app/frontend-admin

ARG NPM_REGISTRY=https://registry.npmmirror.com
ARG COMMIT_ID_ARG

ENV VITE_IS_DOCKER=true
ENV VITE_FRONTEND_COMMIT=${COMMIT_ID_ARG}
ENV NODE_OPTIONS=--max-old-space-size=4096

COPY frontend-admin/package.json ./
COPY frontend-admin/packages/ ./packages/
RUN npm config set registry ${NPM_REGISTRY} && \
    npm install --no-package-lock

COPY frontend-admin/ ./
RUN npm install --no-package-lock && npm run build

# Build the app frontend mounted at `/app/`.
FROM ${NODE_BASE_IMAGE} AS frontend-app-builder

WORKDIR /app/frontend-app

ARG NPM_REGISTRY=https://registry.npmmirror.com
ARG COMMIT_ID_ARG

ENV VITE_IS_DOCKER=true
ENV VITE_FRONTEND_COMMIT=${COMMIT_ID_ARG}
ENV NODE_OPTIONS=--max-old-space-size=4096

COPY frontend-app/package.json ./
COPY frontend-app/packages/ ./packages/
RUN npm config set registry ${NPM_REGISTRY} && \
    npm install --no-package-lock

COPY frontend-app/ ./
RUN npm install --no-package-lock && npm run build

# Package all four static applications in one Nginx image.
FROM nginx:1.28.0-alpine

COPY --from=frontend-builder /app/frontend/dist /usr/share/nginx/html
COPY --from=frontend-default-builder /app/frontend-default/dist /usr/share/nginx/html/default
COPY --from=frontend-admin-builder /app/frontend-admin/dist /usr/share/nginx/html/admin
COPY --from=frontend-app-builder /app/frontend-app/dist /usr/share/nginx/html/app
COPY frontend/nginx.conf /etc/nginx/templates/default.conf.template
COPY frontend/docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

ENV MAX_FILE_SIZE_MB=50

EXPOSE 80

ENTRYPOINT ["/docker-entrypoint.sh"]
