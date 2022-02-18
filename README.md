# Knative Docker Registry

This repository demonstrates a few things:
- run a Docker registry as a [Knative](https://knative.dev) Service
- build an application image using [Tekton](https://tekton.dev) and push it
  to that registry
- run that application image as a Knative Service

Why? Couple of reasons:
- show how it's possible to run someting that is not traditionally thought of
  as a "function" as a Knative service
- show how to use Tekton as a replacement for Knative's Build feature, which
  is almost gone now
- show how to use a local registry for times when you just don't want to deal
  with noise like authentication and service accounts - see
  [Some final notes](#some-final-notes)

## Faking it

If you don't have an IKS cluster, don't have good internet connectivity,
or for whatever reason just don't want to actually execute all of the steps
of the demo but you'd still like to see what the demo looks like, simple run:
```bash
$ USESAVED=1 ./demo
```
The demo will pause for each step, just press the spacebar to continue.

This will run the demo code but use the output from a saved run. The output
will look just like you're running it live.

## Demo Setup

This demo assumes you're using
[IBM's Kubernetes Service](https://cloud.ibm.com/kubernetes)
and have Knative installed. If you don't already have this, please see
the instructions
[here](https://cloud.ibm.com/docs/containers?topic=containers-serverless-apps-knative#knative-setup).

As of today Tekton (the replacement for Knative Build) is not installed
by default in IKS's Knative install, it will be soon, but for now you'll
need to install it yourself:

```bash
$ kubectl apply -f https://storage.googleapis.com/tekton-releases/latest/release.yaml
```

No other special setup should be needed, so let's move on to the demo

## Running the demo

You can run the demo manually or via the `demo` script.

If you want to use the `demo` script, where it'll do all of the typing
for you but still execute the commands, just run:

```bash
$ ./demo
```

For those who want to run the demo manually, let's walk through each step
to explain what's going on.

### Uploading our application

First, we need to upload the source code for our application into
Kubernetes so that our build process can use it. In this case we're going
to use a ConfigMap. This has a few nice aspects to it:
- it's easily populated via the `--from-file` option on
  `kubectl create configmap`
- it can be mounted as a volume into a pod fairly easily
- if necessary, I can switch it to use a Secret instead if security is an issue
- can use it with Knative services too at some point if I wanted to. Right now
  Knative doesn't allow generic Volumes to be mounted into Knative Services -
  sadly

Of course there are issues too. It appears it doesn't support directories
and it doesn't preserve file permissions. Ultimately, a Volume would be nice
but for this demo's purposes a ConfigMap will do.

```bash
$ kubectl create cm source --from-file=src
configmap/source created
```

The `--from-file` option will tell kubectl to create an entry in the ConfigMap
for each file in the referenced directory. That's really handy.

If you want you can look at the `Dockerfile` and `main.go` files in the
`src` directory. There's not really anything too special in there so I'll
skip any in-depth discussion of it. Just know that those two files will be used
during the build process.

### Installing the Docker Registry

Now let's create the Docker Registry service that we'll be uploading our
built image into.

```
kn service create hub --image docker.io/registry --port 5000 --min-scale=1 --max-scale=1
```

In this particular Knative Service I wanted to point out a couple of key things.
First, `minScale` and `maxScale` are both set to `1`. This means that Knative
will always have exactly one instance of the `docker.io/registry` image
running. In a perfect world I would let this scale up and down based on the
load, but I can't because there's no Volume mounted into the Service to
acts as its persistence. This means that each time the underlying pod is
deleted, all images stored in this registry will be lost. Likewise, because
there's no Volume, I can't share it across multiple instances of the Service
to handle a large load. Thus, I'm stuck with exactly one instance.

As I mentioned previously, Knative doesn't allow generic Volumes
to be mounted. Why? Because not everyone supports it yet, so therefore they
decided to block it for everyone. A poor decision to me, but that's
the current state of things. If you want to watch how this progresses
you can watch [this](https://github.com/knative/serving/issues/4417).

Finally, notice that it says `--port  5000`. By default the Registry
will listen on port 5000, so I need to tell Knative to route all requests to
this port number. This allows me to continue to use http and https to talk to
the Registry using the normal IKS/Knative networking that's automatically
setup for you.

### Building the application container image

With our Docker Registry Knative Service running, we can now build and upload
our application image into it. The `task.yaml` file contains two resource
definitions that will do this for us. The first one looks like this:

```bash
# Builds an image and push it to registry.
apiVersion: tekton.dev/v1alpha1
kind: TaskRun
metadata:
  name: build-image
spec:
  taskSpec:
    steps:
    - name: build-and-push
      image: gcr.io/kaniko-project/executor:v0.9.0
      args:
      - --destination=${HUB_URL}:443/hello
      - --context=/workspace/workspace
      volumeMounts:
      - name: source
        mountPath: /workspace/workspace
    volumes:
    - name: source
      configMap:
        name: source
```

I'm not going to go into
Tekton too much, for that you can go look at its [docs](https://tekton.dev)
or this
[tutorial](https://developer.ibm.com/tutorials/knative-build-app-development-with-tekton/).
For now, just know that Tekton is a build tool similar to Jenkins, that will
perform a set of tasks that you tell it. The above yaml file defines one
such Task. Tasks can be comprised of one or more "steps" - each is just
the execution of a container image. In this particular case we're running
the `gcr.io/kaniko-project/executor:v0.9.0` image (which is from the
[Kaniko](https://github.com/GoogleContainerTools/kaniko) project and it'll
do the equivalent of a `docker build` followed by a `docker push`.

Notice that I pass in the name/location of the resulting container image
via the `--destination` argument, and I mount our ConfigMap (the application
source code) into the container as a Volume at `/workspace/workspace`. There's
a lot of text in there, but that's the basic gist of what's going on.

The environment variable looking thing (`${HUB_URL}`) will get replaced
for us by the `kapply` command below - it's just a wrapper for `kubectl`
but does environment variable substitutions first.

So, let's do it:

```bash
$ ./kapply task.yaml
taskrun.tekton.dev/build-image created
```

The build process should take about 45 seconds to complete. You can watch
it with this command:

```bash
$ kubectl get taskrun/build-image -w
```

When you see something that looks like this (SUCCEEDED is `True`):

```bash
NAME          SUCCEEDED   REASON   STARTTIME   COMPLETIONTIME
build-image   True                 55s         1s
```

then it's done and you can stop it by pressing ctrl-c. If something appears to
have gone wrong, look at the TaskRun to get more info with this command:

```bash
$ kubectl get taskrun/build-image -o yaml
```

As implied, this Task will build your image (using the `Dockerfile`)
and then upload it to the Docker Regsitry Knative Service that's running
locally in your IKS cluster.

Just to clean-up some stuff, let's delete everything we've created that
we no longer need:

```bash
$ kubectl delete cm/source
$ kubectl delete -f task.yaml
$ kubectl delete buildtemplate/kaniko
```

### Deploying the application

Now we can deploy the application:

```
$ kn service create hello --image hub-default.kndemo.us-south.containers.appdomain.cloud:443/hello
Service 'hello' successfully created in namespace 'default'.
Waiting for service 'hello' to become ready ... OK

Service URL:
http://hello-default.kndemo.us-south.containers.appdomain.cloud
```

Nothing too exciting here other than the `image` argument points to our
newly created image that's stored in our local Docker Registry. Make sure
the image name here matches the image name from the Task used to build it.

### Testing the application

Then we can test our app using te URL shown in the output above:

```bash
$ curl -s http://hello-default.kntest.us-south.containers.appdomain.cloud
4fh4n: Hello World!
```

All done, so let's clean-up:

```bash
$ kubectl delete ksvc/hub ksvc/hello
```

## Some final notes

As I mentioned previously, I picked Docker's Registry for a couple of
reasons. Most notably, it allows people to use a local Docker Registry
without the need to complicate demos/workshops with authentication, secrets
and service accounts. Clearly, those are all needed in real-world environments
but when you're trying to teach someone about Knative, those things just
complicate matters and are a distraction.

I also liked the idea of using the Docker Registry because it allows me to
show-off using a non-traditional serverless workload. Granted, it would have
been nice if the Registry supported scaling while using a shared Volume
to make the point that Knative should support generic Volumes, but
if I really wanted to scale things I could have used an external storage
service as described in their
[docs](https://docs.docker.com/registry/deploying/).

However, having said all of that, if you want a local Registry and can live
with a single instance (that support multiple threads), perhaps you're
on-prem, then this solution might work just fine for you.... if Knative
supported Volumes so that you didn't lose all of your images if the system
rebooted.

One thing I didn't hook-in yet are
[Registry Notifications](https://docs.docker.com/registry/notifications/).
I haven't played with this yet, but it looks like we should be able to get
notifications about image updates that could then cause a new Knative
Service Revision to be deployed - just like the old Knative Build feature
supported. Perhaps I'll work on that one next...

If you have any comments or questions, ping me.
