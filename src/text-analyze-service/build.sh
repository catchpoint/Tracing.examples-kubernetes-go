cd text-analyze-service

go get .
go mod tidy
go build -o app main.go

cd ..