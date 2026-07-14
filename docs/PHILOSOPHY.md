# Manifest

_If a change conflicts with something here, update this document in the same PR. The reasoning matters as much as the decision._

## Philosophy

> Write programs that do one thing and do it well.
> Write programs to work together.
> Write programs to handle text streams, because that is a universal interface.
>
> — Doug McIlroy, inventor of the Unix pipe (1978)

UNIX made computing powerful by building small, composable tools that combine into systems larger than any single program. `grep`, `sed`, `awk`, `curl` don't try to own a workflow. They do one thing well and disappear into your pipeline.

Language models are powerful but most AI tools wrap them in platforms: chat UIs, agent loops, etc. They capture the user.

`clai` takes a different approach. It brings AI into the pipeline as a filter: text in, text out. The tool serves the user, not the other way around.

## A filter, nothing more

Text goes in, gets transformed, comes out. Everything else is optional.

## Mechanism, not policy

The engine transforms text. It doesn't care about the prompts, which model you use, or what the output looks like. Those decisions are yours.

## Knowledge in data, not code

Behaviors are text files, not compiled features. Adding something new means writing prose. You don't need to be a programmer to contribute or extend its behavior.

## Don't rebuild the shell

> _Don't insist on interactive input._ — McIlroy (1978)

Run, output, exit. No REPL, no wizard, no "press any key." Your terminal is yours.

The shell already writes files, appends, copies to clipboard, loops. Adding flags for things it already does just teaches tool-specific tricks when universal ones exist.

## Quiet on success, loud on failure

Tools that shut up when things go right let downstream tools trust what they receive.

Results go to `stdout`. Diagnostics go to `stderr`. Silence means it worked.

If something breaks, exit immediately with a non-zero code. Errors get one line on `stderr`. Name the problem.

## Small is beautiful

A small surface fits in your head. Every new concept has to earn its place. Most don't.

## Prompts are contributions

The community contributes text files. No registry, no build step, no package manager. Sharing a prompt means hosting a directory.

## POSIX by default

Follow conventions. If you know UNIX tools, you already know how this one works. Surprise costs; familiarity is free.

## References

Essential reading on the philosophy behind this project.

- Ritchie, D. M. & Thompson, K. (1974). [_The UNIX Time-Sharing System_](https://people.eecs.berkeley.edu/~brewer/cs262/unix.pdf). Communications of the ACM, 17(7).
- McIlroy, M. D.; Pinson, E. N.; Tague, B. A. (1978). [_UNIX Time-Sharing System: Foreword_](https://archive.org/details/bstj57-6-1899/page/n3/mode/2up). Bell System Technical Journal, 57(6), pp. 1902–1903.
- Kernighan, B. W. & Pike, R. (1984). _The UNIX Programming Environment_. Prentice Hall.
- Pike, R. & Kernighan, B. W. (1984). [_Program Design in the UNIX Environment_](https://harmful.cat-v.org/cat-v/unix_prog_design.pdf). AT&T Bell Labs Technical Journal, 63(8).
- Gancarz, M. (1994). _The UNIX Philosophy_. Digital Press.
- Salus, P. H. (1994). _A Quarter Century of Unix_. Addison-Wesley.
- Raymond, E. S. (2003). [_The Art of Unix Programming_](http://www.catb.org/~esr/writings/taoup/html/). Addison-Wesley.
