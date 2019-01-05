FROM alpine:3.8 as build

RUN apk update \
    && apk add make rsync python3 bash curl \
    && pip3 install csscompressor htmlmin jinja2

COPY ./ ./app
WORKDIR ./app
RUN make build-dist

FROM nginx:mainline-alpine

MAINTAINER Taahir Ahmed "ahmed.taahir@gmail.com"

COPY --from=build ./app/dist /var/www
COPY --from=build ./app/nginx.conf /etc/nginx/conf.d/default.conf

CMD ["nginx", "-g", "daemon off;"]