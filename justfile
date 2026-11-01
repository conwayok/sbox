install:
	go build -o out/sbox . && \
		cp out/sbox ~/.local/bin/sbox
