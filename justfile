install:
	go build -o out/sbox . && mkdir -p ~/.local/bin && cp out/sbox ~/.local/bin/sbox
