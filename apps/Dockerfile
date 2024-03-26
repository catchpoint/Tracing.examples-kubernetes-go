FROM golang:1.20.1-bullseye
RUN mkdir -p /app
COPY ../src/ /app/src
WORKDIR /app/src

EXPOSE 5000 5000

CMD ["sh", "-c", "./$APP_PATH/build.sh && cd $APP_PATH && ./run.sh"]
