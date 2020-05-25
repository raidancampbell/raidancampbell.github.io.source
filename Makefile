all: deploy

deploy:
	cd public && git checkout master && cd ..

	# Build the project.
	hugo -t beautifulhugo

	# Go To Public folder
	cd public && git add . && git commit -m "rebuilding site $(date)" && git push origin master
