package toolrun

// RunWithWaitDelay is run, exported so black-box tests can shorten the wait
// delay without the package keeping a mutable variable for it.
var RunWithWaitDelay = run
