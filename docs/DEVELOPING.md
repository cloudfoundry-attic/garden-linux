# Development Setup and Workflow

## Setting up Go

Install Go 1.3 or later. For example, install [gvm](https://github.com/moovweb/gvm) and issue:

```
# gvm install go1.3
# gvm use go1.3
```

Extend `$GOPATH` and `$PATH`:

```
# export GOPATH=~/go:$GOPATH
# export PATH=~/go/bin:$PATH
```

## Get garden-linux and its dependencies

```
# go get github.com/cloudfoundry-incubator/garden-linux
# cd ~/go/src/github.com/cloudfoundry-incubator/garden-linux
```

## Install Concourse and Fly

- Install concourse following the instructions at [Concourse.ci](http://concourse.ci)
- Install fly as follows:

```
go get github.com/concourse/fly
```

## Run the Integration Tests

To test under `fly` run

```bash
scripts/garden-fly
```

in the repository root.

`garden-fly` provides the necessary parameters to `fly` which uses `build.yml`
and runs `scripts/concourse-test` on an existing Concourse instance which must
already be running locally in a virtual machine.

## Coding Conventions

Thankfully Go defines a standard code format so we simply adhere to that.

If you write Go code using some tool, like IntelliJ IDEA, which does not enforce
the standard format, you should install a
[pre-commit hook](https://golang.org/misc/git/pre-commit) to check the formatting.

### Error Handling

We generally prefer Go idioms for error handling: the difficulty, particularly
for newcomers, is knowing what those idioms are. So it would be more accurate to
say that we _assert_ the following Go idioms. :-)

[Effective Go](https://golang.org/doc/effective_go.html#errors) doesn't say a
great deal on this subject, but its philosophy is to provide some indication of
the _context_ in which the error occurred (typically the operation or package
that generated the error) and any _symptoms_ the user might find helpful in
correcting the error. The reader could be forgiven for assuming that whenever
symptoms are to be included in an error, a specific type conforming to `error`
should be provided, but this is often overkill (especially when the caller is
unlikely to need programmatic access to the symptoms).

In general it is better to recover from an error, possibly after logging it, and
continue normally so the user doesn't see the error. Do not automatically "wrap,
return and forget" an error without considering recovering from the error.

### Error Messages

An error message should be crafted such that it clearly communicates to the
user what was happening when the failure occured, and the nature of the
failure. Include helpful information in the message, for example if a file
cannot be opened list the filename in question.

In general, we prefer the following format for error messages:

```
{package where error occured}: {attempted behaviour and any additional helpful information}: {underlying error if applicable helpful}
```

For example, the following error from the `link` package follows these
guidelines:

```go
fmt.Errorf("link: create link: invalid number of fds, need 3, got %d", len(fds)")
```

Sometimes the package name alone gives enough indication of the operation in
question, in which case you can be more succinct:

```go
fmt.Errorf("link: invalid number of fds, need 3, got %d", len(fds)")
```

### Wrapping an Underlying Error with a New Message

When wrapping an underlying error, place the underlying error message at the
end, and add any additional context (including the wrapping package) which
is helpful:

```go
fmt.Errorf("devices: create bridge with name '%s': %v", name, err)
```

Listing the activity being undertaken when the error occurred (as above) is
often useful in cases where an error is being wrapped.

Note that when wrapping an underlying error, the underlying error message will
generally already have specified that it failed, so the error message will
often read better if the wrapping message does not *also* begin with a phrase
like 'failed to', for example compare "network: failed to set mtu: failed to
find interface X" to "networking: setting mtu: failed to find interface X".

It is sometimes appropriate to return the underlying error unmodified,
where knowledge of the intermediate function or package does not add
context that would help the user. In these cases it is important to log the
error so that the call chain can be found in the log when debugging (see
'Error Messages vs. Logging' below).

Where a caller of a function may need to take action based on the specific
error that occurred, a rich error type (a type implementing the Error
interface) is helpful. The preferred message format is the same as
above.

```go
type MTUError struct {
	Cause error
	Intf  *net.Interface
	MTU   int
}

func (err MTUError) Error() string {
	return fmt.Spintf("network: set mtu on interface '%v' to %d: %v", err.Intf, err.MTU, err.Cause)
}
```

### Logging Errors

Try not to make error messages a substitute for logging:
the point of an error message is to help the user. the point of logging is to
help maintainers determine where in the code a problem occurred.

Log important errors (along with helpful program state) when they
occur so that unexpected errors can be quickly diagnosed and debugged.

Hence, an error message should not read like
a stack trace: if it does, consider using `panic` instead.
