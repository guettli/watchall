# Watch all Resources in a Kubernetes Cluster

`watchall` is a tool which records changes to Kubernetes resources.

At the moment I use it mostly to record how conditions of CRDs change over time.

# Development

```
❯ wgo -file=.go -file=.templ -xfile=_templ.go templ generate :: go run main.go
```

# TODO

See TODO.md

# how to visualize the many yaml files?

Diff the yaml or diff the html created from a yaml file?

# Timeline

Show every change of every resource by time. Scroll vertical or scroll horizontal?

I think scroll up/down like the git-tree usually gets shown.

In a mgt-cluster with one small wl-cluster are roughly 500 resources.

Hide all resources which have not changed during the recording.

# Analyzing: Set markers

During analyzing the output (going through the history), I want to be able set markers.

For example the timestamp that the cluster was deleted because
the timeout was reached.

# Analyze: Search

What do you want to search for?

* Kind (for example Deployment)
* Name of a resource
* Fulltext

Maybe via k=foo n=foo t=foo

--> CEL?

Or store data in SQLight.

I think SQLight would be better.

Flat table: All resources are in one table.

Deduplication? Not now, optimize later.


# Collection: During e2e test

We use envTest, so how to integrate there?

