# Use the official Golang image as the base image
FROM golang:1.23

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.mod

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the Go application
RUN go build -o reverse-proxy  ./cmd/main.go 

# Expose the port the app runs on
EXPOSE 8888
