# Building stage
FROM docker.m.daocloud.io/golang:1.24

WORKDIR /go/src/github.com/guyuxiang/projectc-custodial-wallet

COPY . /go/src/github.com/guyuxiang/projectc-custodial-wallet

ENV CGO_ENABLED 0
ENV GOOS linux
ENV GO111MODULE=on

ENV TIMEZONE "Asia/Shanghai"

RUN make src.build
RUN mv build/pkg/cmd/projectc-custodial-wallet/projectc-custodial-wallet /go/bin

# Production stage
FROM docker.m.daocloud.io/golang:1.24
WORKDIR /go/bin

LABEL maintainer="guyuxiang <gu_yuxiang@qq.com>"

# copy the go binaries from the building stage
COPY --from=0 /go/bin /go/bin

COPY etc/projectc-custodial-wallet.yaml /etc/projectc-custodial-wallet/projectc-custodial-wallet.yaml

EXPOSE 80

ENTRYPOINT ["./projectc-custodial-wallet"]
