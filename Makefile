DEPS = sus.go go.mod go.sum

.PHONY:
all: bin/susm bin/susd bin/susex

.PHONY:
clean:
	rm -rf bin

bin/susm: susm/main.go $(DEPS) | bin
	go get
	go build -o $@ $<

bin/susd: susd/main.go $(DEPS) | bin
	go get
	go build -o $@ $<

bin/susex: susex/main.go $(DEPS) | bin
	go get
	go build -o $@ $<

bin:
	mkdir bin
