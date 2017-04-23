build:
	go build

install:
	go install

release:
	git tag -a ${version} -m "Release ${version}"
	git push --follow-tags
