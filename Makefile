all: deploy

deploy:
	# If a command fails then the deploy stops
	set -e
	printf "\033[0;32mDeploying updates to GitHub...\033[0m\n"
	# Build the project.
	hugo -t beautifulhugo

	# Go To Public folder
	cd public && git add . && git commit -m "rebuilding site $(date)" && git push origin master
