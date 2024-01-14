# Stage 1: Build stage
FROM golang:1.17.1 AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy the local package files to the container's working directory
COPY . .

# Build the Go application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o paota .

# Stage 2: Production stage
FROM alpine:latest

# Set the working directory inside the container
WORKDIR /app

# Copy only the necessary files from the build stage
COPY --from=builder /app/paota .

# Expose the port the application will run on
EXPOSE 8080

# Command to run the executable
CMD ["./paota"]