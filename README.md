# raidancampbell.com sources

This repository holds the source code for generating raidancampbell.com. 
Human-made things live in the [`content` directory][content dir]: 
[media][media dir] contains any images and applicable sources, 
[posts][posts dir] contains the blog posts themselves, and
[scratch][scratch dir] contains source code used in the blog post. 

As of 2020-06-04, the blog is powered by [Hugo][hugo], 
and themed with a [fork][beautifulhugo fork] off [beautifulhugo][beautifulhugo].

The Makefile handles updating the actual site: the github.io repo is a submodule in the `public` directory. 
Executing `make` will regenerate the static site with `hugo` and commit and push the submodule changes.



[content dir]:https://github.com/raidancampbell/raidancampbell.github.io.source/tree/master/content
[media dir]:https://github.com/raidancampbell/raidancampbell.github.io.source/tree/master/content/media
[posts dir]:https://github.com/raidancampbell/raidancampbell.github.io.source/tree/master/content/posts
[scratch dir]:https://github.com/raidancampbell/raidancampbell.github.io.source/tree/master/content/scratch
[hugo]:https://gohugo.io/
[beautifulhugo fork]:https://github.com/YaguraStation/beautifulhugo/tree/patch-1
[beautifulhugo]:https://github.com/halogenica/beautifulhugo