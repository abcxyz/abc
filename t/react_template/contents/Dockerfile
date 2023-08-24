FROM node:alpine AS webapp-builder
WORKDIR /app
COPY . .

RUN npm install && npm run build --prod --omit=dev

FROM nginx:alpine
COPY --from=webapp-builder /app/build /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 8080
