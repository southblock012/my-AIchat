#构建
FROM node:20-alpine AS builder

WORKDIR /app

COPY ./vue-frontend/package*.json ./

RUN npm ci

COPY ./vue-frontend ./

RUN npm run build

#运行
FROM nginx:alpine

COPY --from=builder /app/dist /usr/share/nginx/html

COPY vue-frontend/nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx","-g","daemon off;"]

