FROM golang:1.19.2-bullseye

RUN mkdir -p /app
COPY ../src/ /app/src
WORKDIR /app/src

EXPOSE 5000 5000

ENV AWS_REGION=us-west-2

CMD ["sh", "-c", "./$APP_PATH/build.sh && cd $APP_PATH && ./run.sh"]
