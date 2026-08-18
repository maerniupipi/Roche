#!/bin/sh

# 生成运行时配置文件，注入环境变量到前端
cat > /usr/share/nginx/html/config.js << EOF
window.__RUNTIME_CONFIG__ = {
  MAX_FILE_SIZE_MB: ${MAX_FILE_SIZE_MB:-50}
};
EOF

# 处理 nginx 配置
export MAX_FILE_SIZE=${MAX_FILE_SIZE_MB}M
export APP_HOST=${APP_HOST:-app}
export APP_PORT=${APP_PORT:-8080}
export APP_SCHEME=${APP_SCHEME:-http}
export FRONTEND_DEFAULT_ENABLED=${FRONTEND_DEFAULT_ENABLED:-true}
export FRONTEND_ADMIN_ENABLED=${FRONTEND_ADMIN_ENABLED:-true}
envsubst '${MAX_FILE_SIZE} ${APP_HOST} ${APP_PORT} ${APP_SCHEME} ${FRONTEND_DEFAULT_ENABLED} ${FRONTEND_ADMIN_ENABLED}' < /etc/nginx/templates/default.conf.template > /etc/nginx/conf.d/default.conf

# 启动 nginx
exec nginx -g 'daemon off;'
