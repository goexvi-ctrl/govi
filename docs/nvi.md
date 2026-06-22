## **Vi/Ex Reference Manual** 

## _Keith Bostic_ 

Computer Science Division Department of Electrical Engineering and Computer Science University of California, Berkeley Berkeley, California 94720 

March 14, 1997 

## _Abstract_ 

This document is the reference guide for the 4.4BSD implementations of **nex** / **nvi** , which are implementations of the historic Berkeley **ex** / **vi** editors. 

## _Licensing_ 

Copyright (c) 1991, 1992, 1993, 1994 

The Regents of the University of California. All Rights Reserved. 

Copyright (c) 1991, 1992, 1993, 1994, 1995, 1996 Keith Bostic. All Rights Reserved. 

The vi program is freely redistributable. You are welcome to copy, modify and share it with others under the conditions listed in the LICENSE file. If any company (not individual!) finds vi sufficiently useful that you would have purchased it, or if any company wishes to redistribute it, contributions to the authors would be appreciated. 

## _Acknowledgements_ 

Bruce Englar encouraged the early development of the historic **ex** / **vi** editor. Peter Kessler helped bring sanity to version 2’s command layout. Bill Joy wrote versions 1 and 2.0 through 2.7, and created the framework that users see in the present editor. Mark Horton added macros and other features and made **ex** / **vi** work on a large number of terminals and Unix systems. 

**Nvi** is originally derived from software contributed to the University of California, Berkeley by Steve Kirkendall, the author of the **vi** clone **elvis** . 

IEEE Standard Portable Operating System Interface for Computer Environments (POSIX) 1003.2 style Regular Expression support was done by Henry Spencer. 

The curses library was originally done by Ken Arnold. Scrolling and reworking for **nvi** was done by Elan Amir. 

George Neville-Neil added the Tcl interpreter, and Sven Verdoolaege added the Perl interpreter. 

Rob Mayoff added Cscope support. 

The Institute of Electrical and Electronics Engineers has given us permission to reprint portions of their documentation. Portions of this document are reprinted and reproduced from IEEE Std 1003.2-1992, IEEE Standard Portable Operating System Interface for Computer Environments (POSIX), copyright 1992 by the Institute of Electrical and Electronics Engineers, Inc. 

**USD:13-4** 


## **1. Description** 

**Vi** is a screen oriented text editor. **Ex** is a line-oriented text editor. **Ex** and **vi** are different interfaces to the same program, and it is possible to switch back and forth during an edit session. **View** is the equivalent of using the **−R** (read-only) option of **vi** . 

This reference manual is the one provided with the **nex** / **nvi** versions of the **ex** / **vi** text editors. **Nex** / **nvi** are intended as bug-for-bug compatible replacements for the original Fourth Berkeley Software Distribution (4BSD) **ex** / **vi** programs. This reference manual is accompanied by a traditional-style manual page. That manual page describes the functionality found in **ex** / **vi** in far less detail than the description here. In addition, it describes the system interface to **ex** / **vi** , e.g. command line options, session recovery, signals, environmental variables, and similar things. 

This reference is intended for users already familiar with **ex** / **vi** . Anyone else should almost certainly read a good tutorial on the editor first. If you are in an unfamiliar environment, and you absolutely have to get work done immediately, see the section entitled “ **Fast Startup** ” in the manual page. It is probably enough to get you started. 

There are a few features in **nex** / **nvi** that are not found in historic versions of **ex** / **vi** . Some of the more interesting of those features are briefly described in the next section, entitled “ **Additional Features** ”. For the rest of this document, **nex** / **nvi** is used only when it is necessary to distinguish it from the historic implementations of **ex** / **vi** . 

Future versions of this software will be periodically made available by anonymous ftp, and can be retrieved from `ftp.cs.berkeley.edu` , in the directory `ucb/4bsd` . 

## **2. Additional Features in Nex/Nvi** 

There are a few features in **nex** / **nvi** that are not found in historic versions of **ex** / **vi** . Some of the more interesting of these are as follows: 

## 

**Nex** / **nvi** will edit any Line lengths are limited by available memory, limited by available disk space. The **vi** text input mode command **<control-X>** can insert any possible character value into the text. 

## **Background and foreground screens** 

The **bg** command backgrounds the current screen, and the **fg** command foregrounds backgrounded screens. The **display** command can be used to list the background screens. 

## **Command Editing** 

You can enter a normal editing window on the collected commands that you’ve entered on the **vi** colon command-line, and then modify and/or execute the commands. See the **cedit** edit option for more information. 

## **Displays** 

The **display** command can be used to display the current buffers, the backgrounded screens, and the tags stack. 

## **Extended Regular Expressions** 

The **extended** option causes Regular Expressions to be interpreted as as Extended Regular Expressions, (i.e. _egrep_ (1) style Regular Expressions). 

## **File Name Completion** 

**vi** colon command-line. See the **filec** option for more information. 

**USD:13-5** 


## 

Changes made during an edit session may be rolled backward and forward. A **.** command immediately after a **u** command continues either forward or backward depending on whether the **u** command was an undo or a redo. 

## **Left-right scrolling** 

The **leftright** option causes **nvi** to do left-right screen scrolling, instead of the traditional **vi** line wrapping. 

## **Message Catalogs** 

It is possible to display informational and error messages in different languages by providing a catalog of messages. See the **msgcat** option and the file `catalog/README` for more information. 

## **Incrementing numbers** 

The **#** command increments or decrements the number referenced by the cursor. 

## 

The **previous** 

## **Scripting languages** 

The **:pe[rl] cmd** , **:perld[o] cmd** and **:tc[l] cmd** commands execute Perl and Tcl/Tk commands, respectively, on lines from the edit buffer. See the “ **Scripting Languages** ” section and the specific commands for more information. 

## **Split screens** 

The **Edit** , **Ex** , **Next** , **Previous** , **Tag** and **Visual** (in **vi** mode) commands divide the screen into multiple editing regions and then perform their normal function in a new screen area. The **<control-W>** command rotates between the foreground screens. The **resize** command can be used to grow or shrink a particular screen. 

## **Tag stacks** 

Tags are now maintained in a stack. The **<control-T>** command returns to the previous tag location. The **tagpop** command returns to the most recent tag location by default, or, optionally to a specific tag number in the tag stack, or the most recent tag from a specified file. The **display** command can be used to list the tags stack. The **tagtop** command returns to the top of the tag stack. 

## **Usage information** 

The **exusage** and **viusage** commands provide usage information for all of the **ex** and **vi** commands by default, or, optionally, for a specific command or key. 

## **Word search** 

The **<control-A>** command searches for the word referenced by the cursor. 

## **3. Startup Information** 

**Ex** / **vi** interprets one of two possible files during startup. The variables and files are expected to contain **ex** commands, not **vi** commands. In addition, they are interpreted _before_ the file to be edited is read, and therefore many **ex** commands may not be used. Generally, any command that requires output to the screen or that needs a file upon which to operate, will cause an error if included in a startup file or environmental variable. 

Because the **ex** command set supported by **nex** / **nvi** is a superset of the command set supported by historical implementations of **ex** , **nex** / **nvi** can use the startup files created for the historical implementations, but the converse may not be true. 

**USD:13-6** 


If the **−s** mental variables and startup files are ignored. 

- (1) `/etc/vi.exrc` is read, as long as it is owned by root or the effective user ID of the user. 

- (2) The environmental variable `NEXINIT` (or the variable `EXINIT` , if `NEXINIT` is not set) is interpreted. 

- (3) If neither `NEXINIT` or `EXINIT` was set, and the `HOME $HOME/.nexrc` (or the file `$HOME/.exrc` , if `$HOME/.nexrc` does not exist) is read, as long 

When the $HOME directory is being used for both **nex** / **nvi** and an historic implementation of **ex** / **vi** , a possible solution is to put **nex** / **nvi** specific commands in the `.nexrc` file, along with a **:source $HOME/.exrc** command to read in the commands common to both implementations. 

- (4) If the **exrc** `.nexrc` (or the file `.exrc` , if `.nexrc` does not exist) is read, as long as the effective user ID of 

edited is read in (or a temporary file is created). Then, any commands specified using the **−c** option are executed, in the context of that file. 

## **4. Recovery** 

There is no recovery program for **nex** / **nvi** , nor does **nex** / **nvi** run setuid. readable and writable by the owner only. Users may recover any file which they can read, and the superuser may recover any edit session. 

**recdir** option (the directory `/var/tmp/vi.recover` by default), and are named “ **vi.XXXXXX** ”, where “ **XXXXXX** ” is a number related to the process ID. When a file is first modified, a second recovery file containing an email message for the user is created, and is named “ **recover.XXXXXX** ”, where, again, “ **XXXXXX** ” is associated with the process ID. Both files are removed at the end of a normal edit session, but will remain if the edit session is abnormally terminated or the user runs the **ex preserve** command. 

The **recdir** option may be set in either the user’s or system’s startup information, changing the recovery directory. (Note, however, that if a memory based file system is used as the backup directory, each system reboot will delete all of the recovery files! The same caution applies to directories such as `/tmp` which are cleared of their contents by a system reboot, or `/usr/tmp` which is periodically cleared of old files on many systems.) 

The recovery directory should be owned by root, or at least by a pseudo-user. In addition, if directory “sticky-bit” semantics are available, the directory should have the sticky-bit set so that files may only be removed by their owners. The recovery directory must be read, write, and executable by any user, i.e. mode 1777. 

If the recovery directory does not exist, **ex** / **vi** will attempt to create it. This can result in the recovery directory being owned by a normal user, which means that that user will be able to remove other user’s recovery and backup files. This is annoying, but is not a security issue as the user cannot otherwise access or modify the files. 

the edit session. In addition, it has all of the necessary email headers for _sendmail_ (8). When the system is rebooted, all of the files in `/var/tmp/vi.recover` named “ **recover.XXXXXX** ” should be sent to their owners, 

**USD:13-7** 


by email, using the **−t** option of **sendmail** (or a similar mechanism in other mailers). If **ex** / **vi** receives a hangup (SIGHUP) signal, or the user executes the **ex preserve** command, **ex** / **vi** will automatically email the recovery information to the user. 

If your system does not have the **sendmail** utility (or a mailer program which supports its interface) the source file `nvi/common/recover.c` will have to be modified to use your local mail delivery programs. Note, if **nex** / **nvi** is changed to use another mailer, it is important to remember that the owner of the file given to the mailer is the **nex** / **nvi** user, so nothing in the file should be trusted as it may have been modified in an effort to compromise the system. 

Finally, are created, and unset when they are first modified, e.g. backup files that have no associated email recovery file will have this bit set. (There is also a small window where empty files can be created and not yet have this bit set. This is due to the method in which the files are created.) Such files should be deleted when the system reboots. 

A simple way to do this cleanup is to run the Bourne shell script **recover** , from your `/etc/rc.local` (or other system startup) file. The script should work with the historic Bourne shell, a POSIX 1003.2 shell or the Korn shell. The **recover** script is installed as part of the **nex** / **nvi** installation process. 

Consult the manual page for details on recovering preserved or aborted editing sessions. 

## **5. Sizing the Screen** 

The size of the screen can be set in a number of ways. **Ex** / **vi** takes the following steps until values are obtained for both the number of rows and number of columns in the screen. 

- (1) If the environmental variable `LINES` exists, it is used to specify the number of rows in the screen. 

- (2) If the environmental variable `COLUMNS` exists, it is used to specify the number of columns in the screen. 

- (3) The TIOCGWINSZ _ioctl_ 

- (4) The termcap entry (or terminfo entry on System V machines) is checked for the “li” entry (rows) and the “co” entry (columns). 

- (5) The number of rows is set to 24, and the number of columns is set to 80. 

If a window change size signal (SIGWINCH) is received, the new window size is retrieved using the TIOCGWINSZ _ioctl_ (2) call, and all other information is ignored. 

## **6. Character Display** 

In both **ex** and **vi** _isprint_ (3) are displayed using the local character 

set. 

Non-printable characters, for which _iscntrl_ (3) returns true, and which are less than octal \040, are displayed as the string “ `ˆ<character>` ”, where `<character>` is the character that is the original character’s value offset from the “ `@` ” character. For example, the octal character \001 is displayed as “ `ˆA` ”. If _iscntrl_ (3) returns true for the octal character \177, it is displayed as the string “ `ˆ?` ”. All other characters are displayed as either hexadecimal values, in the form “ `0x<high-halfbyte> ... 0x<low-halfbyte>` ”, or as octal values, in the form “ `\<high-one-or-two-bits> ... \<low-threebits>` ”. The display of unknown characters is based on the value of the **octal** option. 

In **vi** command mode, the cursor is always positioned on the last column of characters which take up more than one column on the screen. In **vi** text input mode, the cursor is positioned on the first column of characters which take up more than one column on the screen. 

## **7. Multiple Screens** 

**Nvi** supports multiple screens by dividing the window into regions. It also supports stacks of screens by permitting the user to change the set of screens that are currently displayed. 

**USD:13-8** 


The **Edit** , **Ex** , **Fg** , **Next** , **Previous** , **Tag** and **Visual** (in **vi** mode) commands divide the current screen into two regions of approximately equal size and then perform their usual action in a new screen area. If the cursor is in the lower half of the screen, the screen will split up, i.e. the new screen will be above the old one. If the cursor is in the upper half of the screen, the new screen will be below the old one. 

editing the same file. Exiting a screen without saving any changes (or explicitly discarding them) is permitted until the last screen editing the file is exited, at which time the changes must be saved or discarded. 

The **resize** command permits resizing of individual screens. Screens may be grown, shrunk or set to an absolute number of rows. 

The **ˆW** command is used to switch between screens. Each **ˆW** moves to the next lower screen in the window, or to the first screen in the window if there are no lower screens. 

The **bg** command “backgrounds” the current screen. The screen disappears from the window, and the rows it occupied are taken over by a neighboring screen. It is an error to attempt to background the only screen in the window. The **display screens** command displays the names of the files associated with the current backgrounded screens in the window. 

The ground. If no file argument is specified, the first screen on the list is foregrounded. By default, foregrounding consists of backgrounding the current screen, and replacing its space in the window with the foregrounded screen. 

**Fg** , will foreground the backgrounded screen in a new screen instead of swapping it with the current screen. 

screen on the list of backgrounded screens takes over the window. 

## **8. Tags, Tag Stacks, and Cscope** 

**Nvi** supports the historic **vi** tag command **<control-]>** , and the historic **ex** tag command **tag** . These commands change the current file context to a new location, based on information found in the `tags` files. If you are unfamiliar with these commands, you should review their description in the **ex** and **vi** commands section of this manual. For additional information on tags files, see the discussion of the **tags** edit option and the system _ctags_ (1) manual page. 

In addition, **nvi** supports the notion of “tags stacks”, using the **<control-T>** command. The **<control-T>** command returns the user to the previous context, i.e., the last place from which a **<control-]>** or **tag** command was entered. These three commands provide the basic functionality which allows you to use **vi** to review source code in a structured manner. 

**Nvi** also provides two other basic **ex** commands for tag support: **tagpop** and **tagtop** . The **tagpop** command is identical to the **<control-T>** command, with the additional functionality that you may specify that modifications to the current file are to be discarded. This cannot be done using the **<control-T>** command. The **tagtop** command discards all of the contexts that have been pushed onto the tag stack, returning to the context from which the first **<control-]>** or **tag** command was entered. 

The historic _ctags_ declaration or structure or string definition. More sophisticated source code tools often provide multiple locations per tag, e.g., a list of the places from which a function is called or a string definition is used. An example of this functionality is the System V source code tool, **cscope** . 

**Cscope** mation as described in the _cscope_ (1) manual page. **Nvi** contains an interface to the **cscope** query language which permits you to query **cscope** and then sequentially step through the locations in the sources files which **cscope** returns. There are two **nvi** commands which support this ability to step through multiple 

**USD:13-9** 


locations. They are the **ex** commands **tagnext** and **tagprev** . The **tagnext** command moves to the next location for the current tag. The **tagprev** command moves to the previous location for the current tag. (See the **tagnext** and **tagprev** command discussion in the **ex** commands section of this manual for more information.) At any time during this sequential walk, you may use the **<control-]>** , **tag** or **cscope** commands to move to a new tag context, and then use the **<control-T>** or **tagpop** commands to return and continue stepping through the locations for this tag. This is similar to the previous model of a simple tag stack, except that each entry in the tag stack may have more than one file context that is of interest. 

Although there is no widely distributed version of _ctags_ tions per tag, **nvi** has been written to understand the obvious extension to the historic tags file format, i.e., more than a single line in the tags file with the same initial tag name. If you wish to extend your **ctags** implementation or other tool with which you build tags files, this extension should be simple and will require no changes to **nvi** . 

The **nvi** and **cscope** interface is based on the new **ex** command **cscope add** , **find** , **help** , **kill** and **reset** . The subcommand **find** itself has eight subcommands: **c** , **d** , **e** , **f** , **g** , **i** , **s** and **t** . 

## 

The **add cscope** database. The standard filename expansions. If **file** is a directory, the file “cscope.out” in that directory is used as the database. 

After **nvi** attaches to a new database, all subsequent **cscope** queries will be asked of that database. The result of any single query is the collection of response to the query from all of the attached databases. 

If the “CSCOPE_DIRS” environmental variable is set when **nvi** is run, it is expected to be a <colon> or <blank>-separated list of **cscope** databases or directories containing **cscope** databases, to which the user wishes to attach. 

## **:cs[cope] f[ind] c|d|e|f|g|i|s|t buffer|pattern** 

The command is the **cscope** query command for **nvi** . For this command, **nvi** queries all attached **cscope** databases for the pattern. If the pattern is a double-quote character followed by a valid buffer name (e.g., `"<character>` ), then the contents of the named buffer are used as the pattern. Otherwise, the pattern is a Regular Expression. 

The resulting from the query, if the query returned at least one result. 

File names returned by the **cscope** query, if not absolute paths, are searched for relative to the directory where the **cscope** database is located. In addition, if the file “cscope.tpath” appears in the same directory as the **cscope** database, it is expected to contain a colon-separated list of directory names where files referenced by its associated **cscope** database may be found. 

The subcommand is one of the following: 

c Find callers of the name. d Find all function calls made from name. e Find pattern. f Find files with name as substring. g Find definition of name. i Find files #including name. s Find all uses of name. t Find assignments to name. 

## **:cs[cope] h[elp] [command]** 

List the **cscope** commands, or optionally list usage help for any single **cscope** command. 

**USD:13-10** 


## **:display c[onnections]** 

Display the list of **cscope** databases to which **nvi** is currently connected. 

## **:cs[cope] k[ill] #** 

**cscope** database. The connection number is the one displayed by the **ex display connections** command. 

## **:cs[cope] r[eset]** 

Disconnect from all attached **cscope** databases. 

Cscope is not freely redistributable software, but is fairly inexpensive and easily available. To purchase a copy of **cscope** , see http://www.att.com/ssg/products/toolchest.html. 

## **9. Regular Expressions and Replacement Strings** 

**ex substitute** , **global** , and **v** commands, and in search patterns. 

The regular expressions supported by **ex** / **vi** are, by default, the Basic Regular Expressions (BRE’s) described in the IEEE POSIX Standard 1003.2. The **extended** option causes all regular expressions to be interpreted as the Extended Regular Expressions (ERE’s) described by the same standard. (See _re_format_ (7) for more information.) Generally speaking, BRE’s are the Regular Expressions found in _ed_ (1) and _grep_ (1), and ERE’s are the Regular Expressions found in _egrep_ (1). 

The following is not intended to provide a description of Regular Expressions. The information here only describes strings and characters which have special meanings in the **ex** / **vi** version of RE’s, or options which change the meanings of characters that normally have special meanings in RE’s. 

- (1) An empty RE (e.g. “ `//` ” or “ `??` ” is equivalent to the last RE used. 

- (2) The construct “ `\<` ” matches the beginning of a word. 

- (3) The construct “ `\>` ” matches the end of a word. 

- (4) The character “ `˜` ” matches the replacement part of the last **substitute** command. 

When the **magic** option is _not_ set, the only characters with special meanings are a “ `ˆ` ” character at the beginning of an RE, a “ `$` ” character at the end of an RE, and the escaping character “ `\` ”. The characters “ `.` ”, “ `*` ”, “ `[` ” and “ `˜` ” are treated as ordinary characters unless preceded by a “ `\` ”; when preceded by a “ `\` ” they reg ain their special meaning. 

Replacement strings are the second part of a **substitute** command. 

The character “ `&` ” (or “ `\&` ” if the **magic** option is _not_ set) in the replacement string stands for the text matched by the RE that is being replaced. The character “ `˜` ” (or “ `\˜` ” if the **magic** option is _not_ set) stands for the replacement part of the previous **substitute** command. It is only valid after a **substitute** command has been performed. The string “ `\#` ”, where “ `#` ” is an integer value from 1 to 9, stands for the text matched by the portion of the RE enclosed in the “ `#` ”’th set of escaped parentheses, e.g. “ `\(` ” and “ `\)` ”. For example, “ `s/abc\(.*\)def/\1/` ” deletes the strings “ `abc` ” and “ `def` ” from the matched pattern. 

The strings “ `\l` ”, “ `\u` ”, “ `\L` ” and “ `\U` ” can be used to modify the case of elements in the replacement string. The string “ `\l` ” causes the next character to be converted to lowercase; the string “ `\u` ” behaves similarly, but converts to uppercase (e.g. `s/abc/\U&/` replaces the string `abc` with `ABC` ). The string “ `\L` ” causes characters up to the end of the string or the next occurrence of the strings “ `\e` ” or “ `\E` ” to be converted to lowercase; the string “ `\U` ” behaves similarly, but converts to uppercase. 

If the entire replacement pattern is “ `%` ”, then the last replacement pattern is used again. 

In **vi** , inserting a `<control-M>` into the replacement string will cause the matched line to be split into two lines at that point. (The `<control-M>` will be discarded.) 

**USD:13-11** 


## **10. Scripting Languages** 

The **nvi** editor currently supports two scripting languages, Tcl/Tk and Perl. (Note that Perl4 isn’t sufficient, and that the Perl5 used must be version 5.002 or later. See the “ **Building Nvi** ” section for more information. 

The scripting language interface is still being worked on, therefore the following information is probably incomplete, probably wrong in cases, and likely to change. See the `perl_api` and `tcl_api` source directories for more information. As a quick reference, the following function calls are provided for both the Perl and Tcl interfaces. The Perl interface uses a slightly different naming convention, e.g. ‘‘viFindScreen’’ is named ‘‘VI::FindScreen’’. 

## 

Return the `screenId` associated `file` . 

## **viAppendLine screenId lineNumber text** 

Append `text` as a new line after line number `lineNumber` , in the screen `screenId` . 

## **viDelLine screenId lineNum** 

Delete the line `lineNumber` from the screen `screenId` . 

## **viGetLine screenId lineNumber** 

Return the line `lineNumber` from the screen `screenId` . 

## **viInsertLine screenId lineNumber text** 

Insert `text` as a new line before line number `lineNumber` in the screen `screenId` . 

## **viLastLine screenId** 

Return the line number of the last line in the screen `screenId` . 

## **viSetLine screenId lineNumber text** 

Change the line `lineNumber` in the screen `screenId text` . 

## **viGetMark screenId mark** 

`mark` from the screen `screenId` . 

## **viSetMark screenId mark line column** 

`mark` to be at line `line` , column `column` , in the screen `screenId` . 

## **viGetCursor screenId** 

Return the current line and column for the cursor in the screen `screenId` . 

## **viSetCursor screenId line column** 

Set the cursor in the screen `screenId line` and `column` . 

## **viMsg screenId text** 

`text` as a vi message in the screen `screenId` . 

## 

Create a new screen. 

## **viEndScreen screenId** 

Exit the screen `screenId` . 

**USD:13-12** 


## **viSwitchScreen screenId screenId** 

Switch from the screen `screenId` to the screen `screenId` . 

## **viMapKey screenId key tclproc** 

`key` in the screen `screenId` to the Tcl procedure `tclproc` . 

## **viUnmMapKey screenId key** 

`key` in the screen `screenId` 

## **viGetOpt screenId option** 

`option` from the screen `screenId` . 

## **viSetOpt screenId command** 

Set one or more options in the screen `screenId` . 

## **11. General Editor Description** 

When **ex** or **vi** ing changes happen within the context of the copy of the file. _No changes affect the actual file until the file is written out_ , either using a write command or another command which is affected by the **autowrite** option. 

(2) or _fcntl_ (2) interfaces) during the edit session, to avoid inadvertently making modifications to multiple copies of the file. If a lock cannot be obtained for a file because it is locked by another process, the edit session is read-only (as if the **readonly** option or the **−R** flag had been specified). If a lock cannot be obtained for other reasons, the edit session will continue, but the file status information (see the **<control-G>** command) will reflect this fact. 

Both **ex** and **vi** are modeful editors, i.e. they hav e two modes, “command” mode and “text input” mode. The former is intended to permit you to enter commands which modifies already existing text. The latter is intended to permit you to enter new text. When **ex** first starts running, it is in command mode, and usually displays a prompt (see the **prompt** option for more information). The prompt is a single colon (“ `:` ”) character. There are three commands that switch **ex** into text input mode: **append** , **change** and **insert** . Once in input mode, entering a line containing only a single period (“ `.` ”) ends text input mode and returns to command mode, where the prompt is redisplayed. 

When **vi** There are eleven commands that switch **vi** into text input mode: **A** , **a** , **C** , **c** , **I** , **i** , **O** , **o** , **R** , **S** and **s** . Once in input mode, entering an `<escape>` character ends text input mode and returns to command mode. 

**Ex** / **vi Ex** presents a line oriented interface. **Vi** presents a full screen display oriented interface, also known as “visual mode”. In addition, there is a third mode, “open mode”, which is line oriented, but supports cursor movement and editing within the displayed line, similarly to visual mode. Open mode is not yet implemented in **nvi** . 

The following words have special meanings in both the **ex** and **vi** command descriptions: 

## **<interrupt>** 

The interrupt character is used to interrupt the current operation. Normally `<control-C>` , whatev er character is set for the current terminal is used. 

## **<literal-next>** 

The literal next character is used to escape the subsequent character from any special meaning. This character is always `<control-V>` . If the terminal is not set up to do XON/XOFF flow control, then `<control-Q>` is used to mean literal next as well. 

**USD:13-13** 


## **current pathname** 

When the percent character (“ `%` ”) appears in a file name entered as part of an **ex** command argument, it is replaced by the current pathname. (The “ `%` ” character can be escaped by preceding it with a backslash.) 

## **alternate pathname** 

**ex** command, or, the previous current pathname if the last file mentioned becomes the current file. When the hash mark character (“ `#` ”) appears in a file name entered as part of an **ex** command argument, it is replaced by the alternate pathname. (The “ `#` ” character can be escaped by preceding it with a backslash.) 

## **buffer** 

One of a number of named areas for saving copies of text. Commands that change or delete text can save the changed or deleted text into a specific buffer, for later use, if the command allows it (i.e. the **ex change** command cannot save the changed text in a named buffer). Buffers are named with a single character, preceded by a double quote, e.g. `"<character>` in **vi** and without the double quote, e.g. `<character>` , in **ex** . (The double quote isn’t necessary for **ex** because buffers names are denoted by their position in the command line.) Historic implementations of **ex** / **vi** limited `<character>` to the alphanumeric characters; **nex** / **nvi** permits the use of any character without another meaning in the position where a buffer name is expected. 

Buffers named by uppercase characters are the same as buffers named by lowercase characters, e.g. the buffer named by the English character “ `A` ” is the same as the buffer named by the character “ `a` ”, with the exception that, if the buffer contents are being changed (as with a text deletion or **vi change** command), the text is _appended_ to the buffer, instead of replacing the current contents. 

The buffers named by the numeric characters (in English, “ `1` ” through “ `9` ”), are special. If a region of text including characters from more than one line, or a single line of text specified by using a lineoriented motion, is changed or deleted in the file using the **vi change** or **delete** commands, a copy of the text is placed into the numeric buffer “ `1` ”, regardless of the user specifying another buffer in which to save it. In addition, there are a few commands which, when used as a `motion` with the **vi change** and **delete** commands, _always_ copy the specified region of text into the numeric buffers regardless of the region including characters from more than one line. These commands are: 

```
<control-A> % ( )
‘<character> / ? N
n{}
```

Before this copy is done, the previous contents of buffer “ `1` ” are moved into buffer “ `2` ”, “ `2` ” into buffer “ `3` ”, and so on. The contents of buffer “ `9` ” are discarded. In **vi** , text may be explicitly stored into the numeric buffers. In this case, the buffer rotation described above occurs before the replacement of the buffer’s contents. The numeric buffers are only available in `visual` and `open` modes, and are not accessible by **ex** in any way, although changed and deleted text is still stored there while in **ex** mode. 

When a **vi** command synopsis shows both a `[buffer]` and a `[count]` , they may be presented in any order. 

Finally, all buffers are either “line” or “character” oriented. All **ex** commands which store text into buffers are line oriented. Some **vi** commands which store text into buffers are line oriented, and some are character oriented; the description for each applicable **vi** command notes whether text copied into 

**USD:13-14** 


buffers using the command is line or character oriented. In addition, the **vi** command **display buffers** displays the current orientation for each buffer. Generally, the only importance attached to this orientation is that if the buffer is subsequently inserted into the text, line oriented buffers create new lines for each of the lines they contain, and character oriented buffers create new lines for any lines _other_ than the first and last lines they contain. The first and last lines are inserted into the text at the current cursor position, becoming part of the current line. If there is more than one line in the buffer, howev er, the current line itself will be split. 

## **unnamed buffer** 

The unnamed buffer is a text storage area which is used by commands that use or operate on a buffer when no buffer is specified by the user. If the command stores text into a buffer, the text is stored into the unnamed buffer even if a buffer is also specified by the user. It is not possible to append text to the unnamed buffer. If text is appended to a named buffer, the named buffer contains both the old and new text, while the unnamed buffer contains only the new text. There is no way to explicitly reference the unnamed buffer. 

Historically, the contents of the unnamed buffer were discarded by many different commands, even ones that didn’t store text into it. **Nex** / **nvi** never discards the contents of the unnamed buffer until new text replaces them. 

## **whitespace** 

The characters <tab> and <space>. 

## **<carriage-return>** 

The character represented by an ASCII `<control-M>` . This character is almost always treated identically to a `<newline>` character, but differs in that it can be escaped into the file text or into a command. 

## **<newline>** 

The character represented by an ASCII `<control-J>` . This character is almost always treated identically to a `<control-M>` character, but differs in that it cannot be escaped into the file text or into a command. 

## **12. Vi Description** 

**Vi** The bottom line of the screen is used to enter **ex** commands, and for **vi** error and informational messages. If no other information is being displayed, the default display can show the current cursor row and cursor column, an indication of whether the file has been modified, and the current mode of the editor. See the **ruler** and **showmode** options for more information. 

Empty lines do not have any special representation on the screen, but lines on the screen that would logically come after the end of the file are displayed as a single tilde (“ `˜` ”) character. To differentiate between empty lines and lines consisting of only whitespace characters, use the **list** option. Historically, implementations of **vi** have also displayed some lines as single asterisk (“ `@` ”) characters. These were lines that were not correctly displayed, i.e. lines on the screen that did not correspond to lines in the file, or lines that did not fit on the current screen. **Nvi** never displays lines in this fashion. 

**Vi** is a modeful editor, i.e. it has two modes, “command” mode and “text input” mode. When **vi** starts, it is in command mode. There are several commands that change **vi** into text input mode. The `<escape>` character is used to resolve the text input into the file, and exit back into command mode. In **vi** command mode, the cursor is always positioned on the last column of characters which take up more than one column on the screen. In **vi** text insert mode, the cursor is positioned on the first column of characters which take up more than one column on the screen. 


**USD:13-15** 

When positioning the cursor to a new tance to the new cursor position. If the new position is close, the screen is scrolled to the new location. If the new position is far away, the screen is repainted so that the new position is on the screen. If the screen is scrolled, it is moved a minimal amount, and the cursor line will usually appear at the top or bottom of the screen. If the screen is repainted, the cursor line will appear in the center of the screen, unless the cursor is sufficiently close to the beginning or end of the file that this isn’t possible. If the **leftright** option is set, the screen may be scrolled or repainted in a horizontal direction as well as in a vertical one. 

A major difference between the historical **vi** presentation and **nvi** is in the scrolling and screen oriented position commands, **<control-B>** , **<control-D>** , **<control-E>** , **<control-F>** , **<control-U>** , **<controlY>** , **H** , **L** and **M** . In historical implementations of **vi** , these commands acted on physical (as opposed to logical, or screen) lines. For lines that were sufficiently long in relation to the size of the screen, this meant that single line scroll commands might repaint the entire screen, scrolling or screen positioning commands might not change the screen or move the cursor at all, and some lines simply could not be displayed, even though **vi** would edit the file that contained them. In **nvi** , these commands act on logical, i.e. screen lines. You are unlikely to notice any difference unless you are editing files with lines significantly longer than a screen width. 

**Vi** keeps track of the currently “most attractive” cursor position. Each command description (for commands that alter the current cursor position), specifies if the cursor is set to a specific location in the line, or if it is moved to the “most attractive cursor position”. The latter means that the cursor is moved to the cursor position that is horizontally as close as possible to the current cursor position. If the current line is shorter than the cursor position **vi** would select, the cursor is positioned on the last character in the line. (If the line is empty, the cursor is positioned on the first column of the line.) If a command moves the cursor to the most attractive position, it does not alter the current cursor position, and a subsequent movement will again attempt to move the cursor to that position. Therefore, although a movement to a line shorter than the currently most attractive position will cause the cursor to move to the end of that line, a subsequent movement to a longer line will cause the cursor to move back to the most attractive position. 

In addition, the **$** command makes the end of each line the most attractive cursor position rather than a specific column. 

Each **vi** command described below notes where the cursor ends up after it is executed. This position is described in terms of characters on the line, i.e. “the previous character”, or, “the last character in the line”. This is to avoid needing to continually refer to on what part of the character the cursor rests. 

The following words have special meaning for **vi** commands. 

## **previous context** 

The position of the cursor before the command which caused the last absolute movement was executed. Each **vi** command described in the next section that is considered an absolute movement is so noted. In addition, specifying _any_ address to an **ex** command is considered an absolute movement. 

## **motion** 

A second **vi** command can be used as an optional trailing argument to the **vi <** , **>** , **!** , **c** , **d** , **y** , and (depending on the **tildeop** option) **˜** commands. This command indicates the end of the region of text that’s affected by the command. The motion command may be either the command character repeated (in which case it means the current line) or a cursor movement command. In the latter case, the region affected by the command is from the starting or stopping cursor position which comes first in the file, to immediately before the starting or stopping cursor position which comes later in the file. Commands that operate on lines instead of using beginning and ending cursor positions operate on all of the lines that are wholly or partially in the region. In addition, some other commands become line oriented depending on where in the text they are used. The command descriptions below note these special cases. 

The following commands may all be used as motion components for **vi** commands: 


**USD:13-16** 

|`<control-A>`|`<control-H>`|`<control-J>`|`<control-M>`|
|---|---|---|---|
|`<control-N>`|`<control-P>`|`<space>`|`$`|
|`%`|`’<character>`|`(`|`)`|
|`+`|`,`|`-`|`/`|
|`0`|`;`|`?`|`B`|
|`E`|`F`|`G`|`H`|
|`L`|`M`|`N`|`T`|
|`W`|`[[`|`]]`|`ˆ`|
||`‘<character>`|`b`|`e`|
|`_`||||
|`f`|`h`|`j`|`k`|
|`l`|`n`|`t`|`w`|
|`{`|`|`|`}`||



**vi** commands that take motion commands, or the count prefix available for the **vi** commands that are used as motion components, may be included and is _always_ considered part of the motion argument. For example, the commands “ `c2w` ” and “ `2cw` ” are equivalent, and the region affected by the **c** command is two words of text. In addition, if the optional count prefix is specified for both the **vi** command and its motion component, the effect is multiplicative and is considered part of the motion argument. For example, the commands “ `4cw` ” and “ `2c2w` ” are equivalent, and the region affected by the **c** command is four words of text. 

## **count** 

A positive number used as an optional argument to most commands, either to give a size or a position (for display or movement commands), or as a repeat count (for commands that modify text). The count argument is always optional and defaults to 1 unless otherwise noted in the command description. 

When a **vi** command synopsis shows both a `[buffer]` and `[count]` , they may be presented in any order. 

## **word** 

Generally, in languages where it is applicable, **vi** recognizes two kinds of words. First, a sequence of letters, digits and underscores, delimited at both ends by: characters other than letters, digits, or underscores, the beginning or end of a line, and the beginning or end of the file. Second, a sequence of characters other than letters, digits, underscores, or whitespace characters, delimited at both ends by: a letter, digit, underscore, or whitespace character, the beginning or end of a line, and the beginning or end of the file. For example, the characters “ `!@#abc$%ˆ` ” contain three words: “ `!@#` ”, “ `abc` ” and “ `$%ˆ` ”. 

Groups of empty lines (or lines containing only whitespace characters) are treated as a single word. 

## **bigword** 

A set of non-whitespace characters preceded and followed by whitespace characters or the beginning or end of the file or line. For example, the characters “ `!@#abc$%ˆ` ” contain one bigword: “ `!@#abc$%ˆ` ”. 

Groups of empty lines (or lines containing only whitespace characters) are treated as a single bigword. 

## **paragraph** 

and continues until either an empty line, section boundary, or the end of the file. 


**USD:13-17** 

Groups of empty lines (or lines containing only whitespace characters) are treated as a single paragraph. 

**paragraphs** option. 

## **section** 

(“ `{` ”) and continues until the next section or the end of the file. 

**sections** option. 

## **sentence** 

ing the previous sentence, paragraph, or section boundary and continues until the end of the file or a period (“ `.` ”) exclamation point (“ `!` ”) or question mark (“ `?` ”) character, followed by either an endof-line or two whitespace characters. Any number of closing parentheses (“ `)` ”), brackets (“ `]` ”), double-quote (“ `"` ”) or single quote (“ `’` ”) characters can appear between the period, exclamation point, or question mark and the whitespace characters or end-of-line. 

Groups of empty lines (or lines containing only whitespace characters) are treated as a single sentence. 

## **13. Vi Commands** 

The following section describes the commands available in the command mode of the **vi** editor. In each entry below, the tag line is a usage synopsis for the command character. In addition, the final line and column the cursor rests upon, and any options which affect the command are noted. 

## **[count] <control-A>** 

Search forward `count` times for the current word. The pace character on or after the current cursor position, and extends up to the next non-word character or the end of the line. The search is literal, i.e. no characters in the word have any special meaning in terms of Regular Expressions. It is an error if no matching pattern is found between the starting posi- 

The **<control-A>** command is an absolute movement. The **<control-A>** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented. 

Line: Set to the line where the word is found. Column: Set to the first character of the word. Options: Affected by the **ignorecase** and **wrapscan** options. 

## **[count] <control-B>** 

Page backward `count` screens. Two lines of overlap are maintained, if possible, by displaying the window starting at line `(top_line - count * window_size) + 2` , where `window_size` is the value of the **window** option. (In the case of split screens, this size is corrected to the current screen size.) It is an error if the movement is past the beginning of the file. 

Line: Set to the last line of text displayed on the screen. Column: Set to the first nonblank character of the line. Options: Affected by the **window** option. 


**USD:13-18** 

## **[count] <control-D>** 

Scroll forward `count` lines. If `count` by the last **<control-D>** or **<control-U>** command. If this is the first **<control-D>** or **<control-U>** command, scroll forward half the number of lines in the screen. (In the case of split screens, the default scrolling distance is corrected to half the current screen size.) It is an error if the movement is past the end of the file. 

Line: Set to the current line plus the number of lines scrolled. Column: Set to the first nonblank character of the line. Options: None. 

## **[count] <control-E>** 

Scroll forward `count` lines, leaving the cursor on the current line and column, if possible. It is an error if the movement is past the end of the file. 

Line: line on the screen. Column: Unchanged unless the current line scrolls off the screen, in which case it is set to the most attractive cursor position. Options: None. 

## **[count] <control-F>** 

Page forward `count` screens. Two lines of overlap are maintained, if possible, by displaying the window starting at line `top_line + count * window_size - 2` , where `window_size` is the value of the **window** option. (In the case of split screens, this size is corrected to the current screen size.) It is an error if the movement is past the end of the file. 

Line: Column: Set to the first nonblank character of the current line. Options: Affected by the **window** option. 

## **<control-G>** 

The information includes the current pathname, the current line, the number of total lines in the file, the current line as a percentage of the total lines in the file, if the file has been modified, was able to be locked, if the file’s name has been changed, and if the edit session is read-only. 

Line: Unchanged. Column: Unchanged. Options: None. 

## **[count] <control-H>** 

## **[count] h** 

Move the cursor back `count` characters in the current line. character in the line. 

The **<control-H>** and **h** commands may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented. 

Line: Unchanged. Column: Set to the `current - count` character, or, the first character in the line if `count` is greater than or equal to the number of characters in the line before the cursor. 


**USD:13-19** 

Options: None. 

## **[count] <control-J>** 

## **[count] <control-N>** 

## **[count] j** 

Move the cursor down `count` lines without changing the current column. It is an error if the movement is past the end of the file. 

The **<control-J>** , **<control-N>** and **j** commands may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is line oriented. 

Line: Set to the current line plus `count` . Column: The most attractive cursor position. Options: None. 

## **<control-L>** 

## **<control-R>** 

Repaint the screen. 

Line: Unchanged. Column: Unchanged. Options: None. 

## **[count] <control-M>** 

## **[count] +** 

Move the cursor down `count` It is an error if the movement is past the end of the file. 

The **<control-M>** and **+** commands may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is line oriented. 

Line: Set to the current line plus `count` . Column: Set to the first nonblank character in the line. Options: None. 

## **[count] <control-P>** 

## **[count] k** 

Move the cursor up `count` lines, without changing the current column. It is an error if the movement is past the beginning of the file. 

The **<control-P>** and **k** commands may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is line oriented. 

Line: Set to the current line minus `count` . Column: The most attractive cursor position. Options: None. 

## **<control-T>** 

Return to the most recent tag context. The **<control-T>** command is an absolute movement. 

Line: Set to the context of the previous tag command. Column: Set to the context of the previous tag command. 


**USD:13-20** 

Options: None. 

## **[count] <control-U>** 

Scroll backward `count` lines. If `count` fied by the last **<control-D>** or **<control-U>** command. If this is the first **<control-D>** or **<controlU>** command, scroll backward half the number of lines in the screen. (In the case of split screens, the default scrolling distance is corrected to half the current screen size.) It is an error if the movement is past the beginning of the file. 

Line: Set to the current line minus the amount scrolled. Column: Set to the first nonblank character in the line. Options: None. 

## **<control-W>** 

Switch to the next lower screen in the window, or, to the window. 

Line: Set to the previous cursor position in the window. Column: Set to the previous cursor position in the window. Options: None. 

## **[count] <control-Y>** 

Scroll backward `count` lines, leaving the current line and column as is, if possible. It is an error if the movement is past the beginning of the file. 

Line: Unchanged unless the current line scrolls off the screen, in which case it is set to the last line of text displayed on the screen. Column: Unchanged unless the current line scrolls off the screen, in which case it is the most attractive cursor position. Options: None. 

## **<control-Z>** 

Suspend the current editor session. and the **autowrite** option is set, the file is written before the editor session is suspended. If this write fails, the editor session is not suspended. 

Line: Unchanged. Column: Unchanged. Options: Affected by the **autowrite** option. 

## **<escape>** 

Execute **ex** commands or cancel partial commands. If an **ex** command is being entered (e.g. **/** , **?** , **:** or **!** ), the command is executed. If a partial command has been entered, e.g. “ `[0-9]*` ”, or “ `[0-9]*[!<>cdy]` ”, the command is cancelled. Otherwise, it is an error. Line: When an **ex** command is being executed, the current line is set as described for that command. Otherwise, unchanged. Column: When an **ex** command is being executed, the current column is set as described for that command. Otherwise, unchanged. Options: None. 

## **<control-]>** 

Push a tag reference onto the tag stack. **tags** option for more information) are 


**USD:13-21** 

searched for a tag matching the current word. The character on or after the current cursor position, and extends up to the next non-word character or the end of the line. If a matching tag is found, the current file is discarded and the file containing the tag reference is edited. 

**<control-]>** command is an absolute movement. 

Line: Set to the line containing the matching tag string. Column: Set to the start of the matching tag string. Options: Affected by the **tags** and **taglength** options. 

## **<control-ˆ>** 

**autowrite** option is set, the file is written out. If this write fails, the command will fail. Otherwise, if the current file has been modified since it was last completely written, the command will fail. 

Line: Column: Set to the column the cursor was on when the file was last edited. Options: Affected by the **autowrite** option. 

## **[count] <space>** 

## **[count] l** 

Move the cursor forward `count` characters without changing the current line. It is an error if the cursor is on the last character in the line. 

The **<space>** and **l** commands may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented. In addition, these commands may be used as the motion components of other commands when the cursor is on the last character in the line, without error. 

Line: Unchanged. Column: Set to the current character plus the next `count` characters, or to the last character on the line if `count` is greater than the number of characters in the line after the current character. Options: None. 

## **[count] ! motion shell-argument(s)<carriage-return>** 

Replace text with results from a shell command. `count` and `motion` arguments as standard input to the program named by the **shell** option, and replace those lines with the output (both standard error and standard output) of that command. 

After the motion is entered, **vi** prompts for arguments to the shell command. 

Within those arguments, “ `%` ” and “ `#` ” characters are expanded to the current and alternate pathnames, respectively. The “ `!` ” character is expanded with the command text of the previous **!** or **:!** commands. (Therefore, the command **!<motion>!** repeats the previous **!** command.) The special meanings of “ `%` ”, “ `#` ” and “ `!` ” can be overridden by escaping them with a backslash. If no **!** or **:!** command has yet been executed, it is an error to use an unescaped “ `!` ” character as a shell argument. The **!** command does _not_ do shell expansion on the strings provided as arguments. If any of the above expansions change the arguments the user entered, the command is redisplayed at the bottom 


**USD:13-22** 

of the screen. 

**Vi** then executes the program named by the **shell** option, with a **−c** (which are bundled into a single argument). 

The **!** 

**!** command will warn you. 

Line: Column: The first column of the replaced text. Options: Affected by the **shell** option. 

## **[count] # #|+|-** 

Increment or decrement the number referenced by the cursor. If the trailing character is a `+` or `#` , the number is incremented by `count` . If the trailing character is a `-` , the number is decremented by `count` . 

A leading “ `0X` ” or “ `0x` ” causes the number to be interpreted as a hexadecimal number. Otherwise, a leading “ `0` ” causes the number to be interpreted as an octal number, unless a non-octal digit is found as part of the number. Otherwise, the number is interpreted as a decimal number, and may have a leading `+` or `-` sign. The current number begins at the first non-blank character at or after the current cursor position, and extends up to the end of the line or the first character that isn’t a possible character for the numeric type. The format of the number (e.g. leading 0’s, signs) is retained unless the new value cannot be represented in the previous format. 

`unsigned long` ”. Similarly, decimal numbers and their result must fit into a “ `signed long` ”. It is an error to use this command when the cursor is not positioned at a number. 

Line: Unchanged. Column: Set to the first character in the cursor number. Options: None. 

## **[count] $** 

Move the cursor to the end of a line. If `count` down `count - 1` lines. 

It is not an error to use the **$** command when the cursor is on the last character in the line or when the line is empty. 

The **$** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented, unless the cursor is at, or before the first nonblank character in the line, in which case it is line oriented. It is not an error to use the **$** command as a motion component when the cursor is on the last character in the line, although it is an error when the line is empty. 

Line: Set to the current line plus `count` minus 1. Column: Set to the last character in the line. Options: None. 


**USD:13-23** 

## **%** 

Move to the matching character. The cursor moves to the parenthesis or curly brace which _matches_ the parenthesis or curly brace found at the current cursor position or which is the closest one to the right of the cursor on the line. It is an error to execute the **%** command on a line without a parenthesis or curly brace. Historically, any `count` specified to the **%** command was ignored. 

The **%** command is an absolute movement. The **%** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented, unless the starting point of the region is at or before the first nonblank character on its line, and the ending point is at or after the last nonblank character on its line, in which case it is line oriented. 

Line: Set to the line containing the matching character. Column: Set to the matching character. Options: None. 

## **&** 

Repeat the previous substitution command on the current line. 

Historically, any `count` **&** command was ignored. 

Line: Unchanged. Column: Unchanged if the cursor was on the last character in the line, otherwise, set to the first nonblank character in the line. 

Options: Affected by the **edcompatible** , **extended** , **ignorecase** and **magic** options. 

## **´<character>** 

## **‘<character>** 

Return to a context marked by the character `<character>` . If `<character>` is the “ `’` ” or “ `‘` ” character, return to the previous context. If `<character>` is any other character, return to the context marked by that character (see the **m** command for more information). If the command is the **´** command, only the line value is restored, and the cursor is placed on the first nonblank character of that line. If the command is the **‘** command, both the line and column values are restored. 

It is an error if the context no longer exists because of line deletion. (Contexts follow lines that are moved, or which are deleted and then restored.) 

The **´** and **‘** commands are both absolute movements. They may be used as a motion component for other **vi** commands. For the **´** command, any text copied into a buffer is line oriented. For the **‘** command, any text copied into a buffer is character oriented, unless it both starts and stops at the first character in the line, in which case it is line oriented. In addition, when using the **‘** command as a motion component, commands which move backward and started at the first character in the line, or move forward and ended at the first character in the line, are corrected to the last character of the line preceding the starting and ending lines, respectively. 

Line: Set to the line from the context. Column: Set to the first nonblank character in the line, for the **´** command, and set to the context’s column for the **‘** command. Options: None. 

## **[count] (** 

Back up `count` sentences. 

The **(** command is an absolute movement. The **(** command may be used as the motion component of 


**USD:13-24** 

other **vi** commands, in which case any text copied into a buffer is character oriented, unless the starting and stopping points of the region are the first character in the line, in which case it is line oriented. If it is line oriented, the starting point of the region is adjusted to be the end of the line immediately before the starting cursor position. 

Line: Set to the line containing the beginning of the sentence. Column: Set to the first nonblank character of the sentence. Options: Affected by the **lisp** option. 

## **[count] )** 

Move forward `count` sentences. 

The **)** command is an absolute movement. The **)** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented, unless the starting point of the region is the first character in the line, in which case it is line oriented. In the latter case, if the stopping point of the region is also the first character in the line, it is adjusted to be the end of the line immediately before it. 

Line: Set to the line containing the beginning of the sentence. Column: Set to the first nonblank character of the sentence. Options: Affected by the **lisp** option. 

## **[count] ,** 

`count` times. Reverse the last **F** , **f** , **T** or **t** command, searching the other way in the line, `count` times. It is an error if a **F** , **f** , **T** or **t** command has not been performed yet. 

The **,** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented. 

Line: Unchanged. Column: Set to the searched-for character for the **F** and **f** commands, before the character for the **t** command and after the character for the **T** command. Options: None. 

## **[count] −** 

`count` times. 

The **-** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is line oriented. 

Line: Set to the current line minus `count` . Column: Set to the first nonblank character in the line. Options: None. 

## **[count] .** 

Repeat the last **vi** repeated command may be a command and motion component combination. If `count` is specified, it replaces _both_ the count specified for the repeated command, and, if applicable, for the repeated motion component. If `count` is not specified, the counts originally specified to the command being repeated are used again. 

As a special case, if the **.** command is executed immediately after the **u** command, the change log is 


**USD:13-25** 

rolled forward or backward, depending on the action of the **u** command. 

Line: Set as described for the repeated command. Column: Set as described for the repeated command. Options: None. 

## **/RE<carriage-return> /RE/ [offset]<carriage-return> ?RE<carriage-return> ?RE? [offset]<carriage-return> N** 

## **n** 

Search forward or backward for a regular expression. The commands beginning with a slash (“ `/` ”) character are forward searches, the commands beginning with a question mark (“ `?` ”) are backward searches. **Vi** prompts with the leading character on the last line of the screen for a string. It then searches forward or backward in the file for the next occurrence of the string, which is interpreted as a Basic Regular Expression. 

The **/** and **?** commands are absolute movements. They may be used as the motion components of other **vi** commands, in which case any text copied into a buffer is character oriented, unless the search started and ended on the first column of a line, in which case it is line oriented. In addition, forward searches ending at the first character of a line, and backward searches beginning at the first character in the line, are corrected to begin or end at the last character of the previous line. (Note, forward and backward searches can occur for both **/** and **?** commands, if the **wrapscan** option is set.) 

`/` ” or “ `?` ” character is followed by a signed offset), the buffer will always be line oriented (e.g. “ `/string/+0` ” will always guarantee a line orientation). 

The **N** command repeats the previous search, but in the reverse direction. The **n** command repeats the previous search. If either the **N** or **n** commands are used as motion components for the **!** command, you will not be prompted for the text of the bang command, instead the previous bang command will be executed. 

Missing RE’s (e.g. “ `//<carriage-return>` ”, “ `/<carriage-return>` ”, “ `??<carriagereturn>` ”, or “ `?<carriage-return>` ” search for the last search RE, in the indicated direction. 

Searches may be interrupted using the `<interrupt>` character. 

Multiple search patterns may be grouped together by delimiting them with semicolons and zero or more whitespace characters, e.g. `/foo/ ; ?bar?` searches forward for `foo` and then, from that location, backwards for `bar` . When search patterns are grouped together in this manner, the search patterns are evaluated left to right with the final cursor position determined by the last search pattern. 

It is also permissible to append a **z** command to the search strings, e.g. `/foo/ z.` searches forward for the next occurrence of `foo` , and then positions that line in the middle of screen. 

Line: Set to the line in which the match occurred. Column: Set to the first character of the matched string. Options: Affected by the **edcompatible** , **extended** , **ignorecase** , **magic** , and **wrapscan** options. 

Move to It is not an error to use the **0** command when the 

**0** 


**USD:13-26** 

The **0** command may be used as the motion component of other **vi** commands, in which case it is an error if the cursor is on the first character in the line, and any text copied into a buffer is character oriented. 

Line: Unchanged. Column: Set to the first character in the line. Options: None. 

**:** 

Execute an **ex** command. **Vi** prompts for an **ex** command on the last line of the screen, using a colon (“ `:` ”) character. The command is terminated by a `<carriage-return>` , `<newline>` or `<escape>` character; all of these characters may be escaped by using a `<literal-next>` character. The command is then executed. 

If the **ex** command writes to the screen, **vi** will prompt the user for a `<carriage-return>` before continuing when the **ex** command finishes. Large amounts of output from the **ex** command will be paged for the user, and the user prompted for a `<carriage-return>` or `<space>` key to continue. In some cases, a quit (normally a “q” character) or `<interrupt>` may be entered to interrupt the **ex** command. 

When the **ex** enter another “ `:` ” character followed by another **ex** command. 

Line: The current line is set as described for the **ex** command. Column: The current column is set as described for the **ex** command. Options: Affected as described for the **ex** command. 

## **[count] ;** 

`count` times. The **F** , **f** , **T** or **t** commands. It is an error if a **F** , **f** , **T** or **t** command has not been performed yet. 

The **;** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented. 

Line: Unchanged. Column: Set to the searched-for character for the **F** and **f** commands, before the character for the **t** command and after the character for the **T** command. 

Options: None. 

## **[count] < motion** 

## **[count] > motion** 

Shift lines left or right. `count` and `motion` left (for the **<** command) or right (for the **>** command) by the number of columns specified by the **shiftwidth** option. Only whitespace characters are deleted when shifting left. Once the first character in the line no longer contains a whitespace character, the command will succeed, but the line will 

Line: Unchanged. Column: Set to the first nonblank character in the line. Options: Affected by the **shiftwidth** option. 


**USD:13-27** 

## **@ buffer** 

Execute a named buffer. Execute the named buffer as **vi** commands. The buffer may include **ex** commands, too, but they must be expressed as a **:** command. If the buffer is line oriented, `<newline>` characters are logically appended to each line of the buffer. If the buffer is character oriented, `<newline>` characters are logically appended to all but the last line in the buffer. 

If the buffer name is “ `@` ”, or “ `*` ”, then the last buffer executed shall be used. It is an error to specify “ `@@` ” or “ `@*` ” if there were no previous buffer executions. The text of a buffer may contain a **@** command, and it is possible to create infinite loops in this manner. (The `<interrupt>` character may be used to interrupt the loop.) 

Line: The current line is set as described for the command(s). Column: The current column is set as described for the command(s). Options: None. 

## **[count] A** 

Enter input mode, appending the text after the end of the line. If `count` repeatedly input `count - 1` more times after input mode is exited. 

Line: Set to the last line upon which characters were entered. Column: Set to the last character entered. Options: Affected by the **altwerase** , **autoindent** , **beautify** , **showmatch** , **ttywerase** and **wrapmargin** options. 

## **[count] B** 

Move backward `count` bigwords. Move the cursor backward to the beginning of a bigword by repeating the following algorithm: if the current position is at the beginning of a bigword or the character at the current position cannot be part of a bigword, move to the first character of the preceding bigword. Otherwise, move to the first character of the bigword at the current position. If no preceding bigword exists on the current line, move to the first character of the last bigword on the first preceding line that contains a bigword. 

The **B** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented. 

Line: Set to the line containing the word selected. Column: Set to the first character of the word selected. Options: None. 

## **[buffer] [count] C** 

Change text from the current position to the end-of-line. If `count` replaces from the current position to the end-of-line, plus `count - 1` subsequent lines. 

Line: Set to the last line upon which characters were entered. Column: Set to the last character entered. Options: Affected by the **altwerase** , **autoindent** , **beautify** , **showmatch** , **ttywerase** and **wrapmargin** options. 

## **[buffer] D** 

Delete text from the current position to the end-of-line. 

It is not an error to execute the **D** command on an empty line. 


**USD:13-28** 

Line: Unchanged. Column: Set to the character before the current character, or, column 1 if the cursor was on column 1. Options: None. 

## **[count] E** 

Move forward `count` end-of-bigwords. Move the cursor forward to the end of a bigword by repeating the following algorithm: if the current position is the end of a bigword or the character at that position cannot be part of a bigword, move to the last character of the following bigword. Otherwise, move to the last character of the bigword at the current position. If no succeeding bigword exists on the current line, move to the last character of the first bigword on the next following line that contains a bigword. 

The **E** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented. 

Line: Set to the line containing the word selected. Column: Set to the last character of the word selected. Options: None. 

## **[count] F <character>** 

Search `count` times backward through the current line for `<character>` . 

The **F** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented. 

Line: Unchanged. Column: Set to the searched-for character. Options: None. 

## **[count] G** 

Move to line `count count` 

The **G** command is an absolute movement. The **G** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is line oriented. 

Line: Set to `count` Column: Set to the first nonblank character in the line. Options: None. 

## **[count] H** 

Move to the screen line `count - 1` lines below the top of the screen. 

The **H** command is an absolute movement. The **H** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is line oriented. 

Line: Set to the line `count - 1` lines below the top of the screen. Column: Set to the first nonblank character of the _screen_ line. Options: None. 

## **[count] I** 

Enter input mode, inserting the text at the beginning of the line. If `count` is repeatedly input `count - 1` more times. 


**USD:13-29** 

Line: Set to the last line upon which characters were entered. Column: Set to the last character entered. Options: None. 

## **[count] J** 

Join lines. If `count count` lines are joined; a minimum of two lines are always joined, regardless of the value of `count` . 

If the current line ends with a whitespace character, all whitespace is stripped from the next line. Otherwise, if the next line starts with a open parenthesis (“ `(` ”) do nothing. Otherwise, if the current line ends with a question mark (“ `?` ”), period (“ `.` ”) or exclamation point (“ `!` ”), insert two spaces. Otherwise, insert a single space. 

Line: Unchanged. Column: Set to the character after the last character of the next-to-last joined line. Options: None. 

## **[count] L** 

Move to the screen line `count - 1` lines above the bottom of the screen. 

The **L** command is an absolute movement. The **L** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is line oriented. 

Line: Set to the line `count - 1` lines above the bottom of the screen. Column: Set to the first nonblank character of the _screen_ line. Options: None. 

## **M** 

Move to the screen line in the middle of the screen. 

The **M** command is an absolute movement. The **M** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is line oriented. 

Historically, any `count` **M** command was ignored. 

Line: Set to the line in the middle of the screen. Column: Set to the first nonblank character of the _screen_ line. Options: None. 

## **[count] O** 

Enter input mode, appending text in a new line above the current line. If `count` input is repeatedly input `count - 1` more times. 

Historically, any `count` **O** command was ignored. 

Line: Set to the last line upon which characters were entered. Column: Set to the last character entered. Options: Affected by the **altwerase** , **autoindent** , **beautify** , **showmatch** , **ttywerase** and **wrapmargin** options. 


**USD:13-30** 

## **[buffer] P** 

Insert text from a buffer. Text from the buffer (the unnamed buffer by default) is inserted before the current column or, if the buffer is line oriented, before the current line. 

Line: Set to the lowest numbered line insert, if the buffer is line oriented, otherwise unchanged. Column: Set to the first nonblank character of the appended text, if the buffer is line oriented, otherwise, the last character of the appended text. Options: None. 

## **Q** 

Exit **vi** (or visual) mode and switch to **ex** mode. 

Line: Unchanged. Column: No longer relevant. Options: None. 

## **[count] R** 

Enter input mode, replacing the characters in the current line. If `count` repeatedly input `count - 1` more times. 

If the end of the current line is reached, no more characters are replaced and any further characters input are appended to the line. 

Line: Set to the last line upon which characters were entered. Column: Set to the last character entered. Options: Affected by the **altwerase** , **autoindent** , **beautify** , **showmatch** , **ttywerase** and **wrapmargin** options. 

## **[buffer] [count] S** 

Substitute `count` lines. 

Line: Set to the last line upon which characters were entered. Column: Set to the last character entered. Options: Affected by the **altwerase** , **autoindent** , **beautify** , **showmatch** , **ttywerase** and **wrapmargin** options. 

## **[count] T <character>** 

Search backward, `count` times, through the current line for the character _after_ `<character>` . 

The **T** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented. 

Line: Unchanged. Column: Set to the character _after_ the searched-for character. Options: None. 

## **U** 

Restore the current line to its state before the cursor last moved to it. 

Line: Unchanged. 


**USD:13-31** 

Options: None. 

## **[count] W** 

Move forward `count` bigwords. Move the cursor forward to the beginning of a bigword by repeating the following algorithm: if the current position is within a bigword or the character at that position cannot be part of a bigword, move to the first character of the next bigword. If no subsequent bigword exists on the current line, move to the first character of the first bigword on the first following line that contains a bigword. 

The **W** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented. 

Line: The line containing the word selected. Column: The first character of the word selected. Options: None. 

## **[buffer] [count] X** 

Delete `count` characters before the cursor. If the number of characters to be deleted is greater than or equal to the number of characters to the beginning of the line, all of the characters before the current cursor position, to the beginning of the line, are deleted. 

Line: Unchanged. Column: Set to the current character minus `count` , or the first character if count is greater than the number of characters in the line before the cursor. Options: None. 

## **[buffer] [count] Y** 

Copy (or “yank”) `count` 

Line: Unchanged. Column: Unchanged. Options: None. 

## **ZZ** 

**vi** . of the file to any file. 

The **ZZ** Entering two “quit” commands (i.e. **wq** , **quit** , **xit** or **ZZ** ) in a row will override this check and the editor will exit, ignoring any files that have not yet been edited. 

Line: Unchanged. Column: Unchanged. Options: None. 

## **[count] [[** 

Back up `count` section boundaries. 

The **[[** command is an absolute movement. The **[[** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented, unless the starting position is column 0, in which case it is line oriented. 


**USD:13-32** 

Line: Set to the previous line that is `count` if no more section boundaries exist preceding the current line. Column: Set to the first nonblank character in the line. Options: Affected by the **sections** option. 

## **[count] ]]** 

Move forward `count` section boundaries. 

The **]]** command is an absolute movement. The **]]** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented, unless the starting position is column 0, in which case it is line oriented. 

Line: Set to the line that is `count` more section boundaries exist following the current line. Column: Set to the first nonblank character in the line. Options: Affected by the **sections** option. 

## **ˆ** 

The **ˆ** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented. 

Line: Unchanged. Column: Set to the first nonblank character of the current line. Options: None. 

## **[count] _** 

Move down `count - 1` The **_** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is line oriented. 

It is not an error to execute the **_** 

Line: The current line plus `count - 1` . Column: The first nonblank character in the line. Options: None. 

## **[count] a** 

Enter input mode, appending the text after the cursor. If `count` edly input `count - 1` more times. 

Line: Set to the last line upon which characters were entered. Column: Set to the last character entered. Options: Affected by the **altwerase** , **autoindent** , **beautify** , **showmatch** , **ttywerase** and **wrapmargin** options. 

## **[count] b** 

Move backward `count` words. Move the cursor backward to the beginning of a word by repeating the following algorithm: if the current position is at the beginning of a word, move to the first character of the preceding word. Otherwise, the current position moves to the first character of the word at the current position. If no preceding word exists on the current line, move to the first character of the 


**USD:13-33** 

The **b** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented. 

Line: Set to the line containing the word selected. Column: Set to the first character of the word selected. Options: None. 

## **[buffer] [count] c motion** 

`count` and `motion` . If only part of a single line is affected, then the last character being changed is marked with a “ `$` ”. Otherwise, the region of text is deleted, and input mode is entered. 

Line: Set to the last line upon which characters were entered. Column: Set to the last character entered. Options: Affected by the **altwerase** , **autoindent** , **beautify** , **showmatch** , **ttywerase** and **wrapmargin** options. 

## **[buffer] [count] d motion** 

`count` and `motion` . 

Line: Set to the line where the region starts. Column: Set to the first character in the line after the last character in the region. If no such character exists, set to the last character before the region. Options: None. 

## **[count] e** 

Move forward `count` end-of-words. Move the cursor forward to the end of a word by repeating the following algorithm: if the current position is the end of a word, move to the last character of the following word. Otherwise, move to the last character of the word at the current position. If no succeeding word exists on the current line, move to the last character of the first word on the next following line that contains a word. 

The **e** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented. 

Line: Set to the line containing the word selected. Column: Set to the last character of the word selected. Options: None. 

## **[count] f <character>** 

Search forward, `count` times, through the rest of the current line for `<character>` . 

The **f** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented. 

Line: Unchanged. Column: Set to the searched-for character. Options: None. 

## **[count] i** 

Enter input mode, inserting the text before the cursor. If `count` 


**USD:13-34** 

repeatedly input `count - 1` more times. 

Line: Set to the last line upon which characters were entered. Column: Set to the last character entered. Options: Affected by the **altwerase** , **autoindent** , **beautify** , **showmatch** , **ttywerase** and **wrapmargin** options. 

## **m <character>** 

Save the current context (line and column) as `<character>` . The exact position is referred to by “ `‘<character>` ”. The line is referred to by “ `’<character>` ”. 

Historically, `<character>` was restricted to lower-case letters. **Nvi** permits the use of any character. 

Line: Unchanged. Column: Unchanged. Options: None. 

## **[count] o** 

Enter input mode, appending text in a new line under the current line. If `count` input is repeatedly input `count - 1` more times. 

Historically, any `count` **o** command was ignored. 

Line: Set to the last line upon which characters were entered. Column: Set to the last character entered. Options: Affected by the **altwerase** , **autoindent** , **beautify** , **showmatch** , **ttywerase** and **wrapmargin** options. 

## **[buffer] p** 

Append text from a buffer. Text from the buffer (the unnamed buffer by default) is appended after the current column or, if the buffer is line oriented, after the current line. 

Line: Column: Set to the first nonblank character of the appended text if the buffer is line oriented, otherwise, the last character of the appended text. Options: None. 

## **[count] r <character>** 

Replace characters. The next `count` characters in the line are replaced with `<character>` . Replacing characters with `<newline>` characters results in creating new, empty lines into the file. 

If `<character>` is `<escape>` , the command is cancelled. 

Line: Unchanged unless the replacement character is a `<newline>` , in which case it is set to the current line plus `count - 1` . Column: Set to the last character replaced, unless the replacement character is a `<newline>` , in which case the cursor is in column 1 of the last line inserted. Options: None. 

## **[buffer] [count] s** 

Substitute `count` characters in the current line starting with the current character. 


**USD:13-35** 

Line: Set to the last line upon which characters were entered. Column: Set to the last character entered. Options: Affected by the **altwerase** , **autoindent** , **beautify** , **showmatch** , **ttywerase** and **wrapmargin** options. 

## **[count] t <character>** 

Search forward, `count` times, through the current line for the character immediately _before_ `<character>` . 

The **t** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented. 

Line: Unchanged. Column: Set to the character _before_ the searched-for character. Options: None. 

## **u** 

If repeated, the **u** command alternates between these two states, and is its own inverse. When used after an insert that inserted text on more than one line, the lines are saved in the numeric buffers. 

The **.** command, when used immediately after the **u** command, causes the change log to be rolled forward or backward, depending on the action of the **u** command. 

Line: Set sents an addition or change; otherwise, the line preceding the deleted text. Column: Set to the cursor position before the change was made. Options: None. 

## **[count] w** 

Move forward `count` words. Move the cursor forward to the beginning of a word by repeating the following algorithm: if the current position is at the beginning of a word, move to the first character of the next word. If no subsequent word exists on the current line, move to the first character of the first word on the first following line that contains a word. 

The **w** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented. 

Line: Set to the line containing the word selected. Column: Set to the first character of the word selected. Options: None. 

## **[buffer] [count] x** 

Delete `count` characters. The deletion is at the current character position. If the number of characters to be deleted is greater than or equal to the number of characters to the end of the line, all of the characters from the current cursor position to the end of the line are deleted. 

Line: Unchanged. Column: Unchanged unless the last character in the line is deleted and the cursor is not already on the first character in the line, in which case it is set to the previous character. Options: None. 


**USD:13-36** 

## **[buffer] [count] y motion** 

`count` and `motion` , into a buffer. 

Line: Unchanged, unless the region covers more than a single line, in which case it is set to the line where the region starts. 

Column: Unchanged, unless the region covers more than a single line, in which case it is set to the character were the region starts. Options: None. 

## **[count1] z [count2] type** 

Redraw the screen with a window `count2` lines long, with line `count1 type` character. If `count1` is not specified, it defaults to the current line. If `count2` is not specified, it defaults to the current window size. 

The following `type` characters may be used: 

+ If `count1 count1` at the top of the screen. Otherwise, display the screen after the current screen, similarly to the **<control-F>** command. <carriage-return> 

Place the line `count1` at the top of the screen. 

- . Place the line `count1` in the center of the screen. 

- Place the line `count1` at the bottom of the screen. 

- ˆ If `count1 count1` is at the bottom of the screen, at the bottom of the screen, i.e. display the screen before the screen before `count1` . Otherwise, display the screen before the current screen, similarly to the **<control-B>** command. 

Line: Set to `count1` unless `count1 type` character was either “ `ˆ` ” or “ `+` ”, in which case it is set to the line before the first line on the previous screen or the line after the last line on the previous screen, respectively. 

Options: None. 

## **[count] {** 

Move backward `count` paragraphs. 

The **{** command is an absolute movement. The **{** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented, unless the starting character is the first character on its line, in which case it is line oriented. 

Line: Set to the line containing the beginning of the previous paragraph. Column: Set to the first nonblank character in the line. Options: Affected by the **paragraph** option. 

## **[count] |** 

_column_ position on the current line. 

The **|** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented. It is an error to use the **|** command as a motion component and for the cursor not to move. 

Line: Unchanged. 


**USD:13-37** 

Column: Set `count` , if the position exists in the line. If the column length of the current line is less than `count` , the cursor is moved to the last character in the line. 

Options: None. 

## **[count] }** 

Move forward `count` paragraphs. 

The **}** command is an absolute movement. The **}** command may be used as the motion component of other **vi** commands, in which case any text copied into a buffer is character oriented, unless the starting character is at or before any nonblank characters in its line, in which case it is line oriented. 

Line: Set to the line containing the beginning of the next paragraph. Column: Set to the first nonblank character in the line. Options: Affected by the **paragraph** option. 

## **[count] ˜** 

Reverse the case of the next `count` character(s). This is the historic semantic for the **˜** command and it is only in effect if the **tildeop** option is not set. 

Lowercase alphabetic characters are changed to uppercase, and uppercase characters are changed to lowercase. No other characters are affected. 

Historically, the **˜** command did not take an associated count, nor did it move past the end of the current line. As it had no associated motion it was difficult to change the case of large blocks of text. In **nvi** , if the cursor is on the last character of a line, and there are more lines in the file, the cursor moves to the next line. 

It is not an error to specify a count larger than the number of characters between the cursor and the 

Line: Set to the line of the character after `count` Column: Set to the character after `count` characters, or, end-of-file. Options: Affected by the **tildeop** option. 

## **[count] ˜ motion** 

`count` and `motion` . Only in effect if the **tildeop** option is set. 

Lowercase characters are changed to uppercase, and uppercase characters are changed to lowercase. No other characters are affected. 

Line: Set to the line of the character after the last character in the region. Column: Set to the character after the last character in the region. Options: Affected by the **tildeop** option. 

## **<interrupt>** 

Interrupt the current operation. Many of the potentially long-running **vi** commands may be interrupted using the terminal interrupt character. These operations include searches, file reading and writing, filter operations and map character expansion. Interrupts are also enabled when running commands outside of **vi** . 

If the `<interrupt>` character is used to interrupt while entering an **ex** command, the command is 


**USD:13-38** 

aborted, the cursor returns to its previous position, and **vi** remains in command mode. 

Generally, if the `<interrupt>` character is used to interrupt any operation, any changes made before the interrupt are left in place. 

Line: Dependent on the operation being interrupted. Column: Dependent on the operation being interrupted. Options: None. 

## **14. Vi Text Input Commands** 

The following section describes the commands available in the text input mode of the **vi** editor. 

Historically, **vi** implementations only permitted the characters inserted on the current line to be erased. In addition, only the `<control-D>` erase character and the “ `0<control-D>` ” and “ `ˆ<control-D>` ” erase strings could erase autoindent characters. (Autoindent characters include both the characters inserted automatically at the beginning of an input line as well as characters inserted using the `<control-T>` command.) This implementation permits erasure to continue past the beginning of the current line, and back to where text input mode was entered. In addition, autoindent characters may be erased using the standard erase characters. For the line and word erase characters, reaching the autoindent characters forms a “soft” boundary, denoting the end of the current word or line erase. Repeating the word or line erase key will erase the autoindent characters. 

Historically, **vi** always used `<control-H>` and `<control-W>` as character and word erase characters, respectively, reg ardless of the current terminal settings. This implementation accepts, in addition to these two characters, the current terminal characters for those operations. 

## **<nul>** 

`<nul>` , the previous input is replayed, as if just entered. 

## **<control-D>** 

If the previous character on the line was an autoindent character, erase characters to move the cursor back to the column immediately after the previous (1-based) column which is a multiple of the **shiftwidth** edit option. This may result in any number of `<tab>` and `<space>` characters preceding the cursor being changed. 

Otherwise, if the **autoindent** `<control-D>` is ignored. Otherwise, a literal `<control-D>` character is entered. 

## **ˆ<control-D>** 

If the previous character on the line was an autoindent character, erase all of the autoindent characters on the line. In addition, the autoindent level is reset to 0. 

## **0<control-D>** 

If the previous character on the line was an autoindent character, erase all of the autoindent characters on the line. The autoindent level is not altered. 

## **<control-T>** 

`<tab>` and `<space>` characters to move the cursor forward to the column immediately after the next (1-based) column which is a multiple of the **shiftwidth** edit option. This may result in any number of `<tab>` and `<space>` characters preceding the cursor being changed. 

Historically, **vi** did not permit the `<control-T>` command to be used unless the cursor was at the first column of a new line or it was preceded only by autoindent characters. **Nvi** permits 


**USD:13-39** 

it to be used at any time during insert mode. 

## **<erase> <control-H>** 

Erase the last character. 

## **<literal-next>** 

Quote the next character. The next character will not be mapped (see the **map** command for more information) or interpreted specially. A carat (“ `ˆ` ”) character will be displayed immediately as a placeholder, but will be replaced by the next character. 

## **<escape>** 

If on the colon command line, and the edit option is set, behave as described for that option. Otherwise, if on the colon command line, execute the command. Otherwise, if not on the colon command line, resolve all text input into the file, and return to command mode. 

## **<line erase>** 

Erase the current line. 

## **<control-W>** 

## **<word erase>** 

Erase the last word. The **altwerase** and **ttywerase** options. 

## **<control-X>[0-9A-Fa-f]+** 

any non-hexadecimal character or the input of the maximum number of characters that can be translated into a single character value. 

## **<interrupt>** 

Interrupt text input mode, returning to command mode. If the `<interrupt>` character is used to interrupt inserting text into the file, it is as if the `<escape>` character was used; all text input up to the interruption is resolved into the file. 

## **15. Ex Addressing** 

Addressing in **ex** (and when **ex** commands are executed from **vi** ) relates to the current line. In general, the current line is the last line affected by a command. The exact effect on the current line is discussed under the description of each command. When the file contains no lines, the current line is zero. 

Addresses are constructed by one or more of the following methods: 

- (1) The address “ `.` ” refers to the current line. 

- (2) The address “ `$` 

- (3) The address “ `N` ”, where `N` 

- (4) The address “ `’<character>` ” or “ `‘<character>` ” refers to the line marked with the name `<character>` . (See the **k** or **m** commands for more information on how to mark lines.) 

- (5) A regular expression (RE) enclosed by slashes (“ `/` line found by searching forward from the line _after_ the current line toward the end of the file, and stopping at the first line containing a string matching the RE. (The trailing slash can be omitted at the end of the command line.) 

**USD:13-40** 


`//` ”, the last RE used in any command is used in the search. 

If the **extended** option is set, the RE is handled as an extended RE, not a basic RE. If the **wrapscan** option is set, the search wraps around to the beginning of the file and continues up to and including the current line, so that the entire file is searched. 

The form “ `\/` ” is accepted for historic reasons, and is identical to “ `//` ”. 

- (6) An RE enclosed in question marks (“ `?` ”) addresses ward from the line _preceding_ the current line, toward the beginning of the file and stopping at the first line containing a string matching the RE. (The trailing question mark can be omitted at the end of a command line.) 

`??` ”, the last RE used in any command is used in the search. 

If the **extended** option is set, the RE is handled as an extended RE, not a basic RE. If the **wrapscan** option is set, the search wraps around from the beginning of the file to the end of the file and continues up to and including the current line, so that the entire file is searched. 

The form “ `\?` ” is accepted for historic reasons, and is identical to “ `??` ”. 

- (7) An address followed by a plus sign (“ `+` ”) or a minus sign (“ `-` ”) followed by a number is an offset address and refers to the address plus (or minus) the indicated number of lines. If the address is omitted, the addition or subtraction is done with respect to the current line. 

- (8) An address of “ `+` ” or “ `−` ” followed by a number is an offset from the current line. For example, “ `−5` ” is the same as “ `.−5` ”. 

- (9) An address ending with “ `+` ” or “ `-` ” has 1 added to or subtracted from the address, respectively. As a consequence of this rule and of the previous rule, the address “ `−` ” refers to the line preceding the current line. Moreover, trailing “ `+` ” and “ `−` ” characters have a cumulative effect. For example, “ `++−++` ” refers to the current line plus 3. 

- (10) A percent sign (“ `%` ”) is equivalent to the address range “ `1,$` ”. 

**Ex** commands require zero, one, or two addresses. It is an error to specify an address to a command which requires zero addresses. 

If the user provides more than the expected number of addresses to any **ex** addresses specified are discarded. For example, “ `1,2,3,5` ”print prints lines 3 through 5, because the **print** command only takes two addresses. 

The addresses in a range are separated from each other by a comma (“ `,` ”) or a semicolon (“ `;` ”). In the latter case, the current line (“ `.` ”) is set to the first address, and only then is the second address calculated. This feature can be used to determine the starting line for forward and backward searches (see rules (5) and (6) above). The second address of any two-address sequence corresponds to a line that follows, in the file, the line corresponding to the first address. The first address must be less than or equal to the second address. The first address must be greater than or equal to the first line of the file, and the last address must be less than or equal to the last line of the file. 

## **16. Ex Description** 

The following words have special meanings for **ex** commands. 

## 

**ex** editor. This character is normally `<control-D>` . Howev er, whatever character is set for the current terminal is supported as 


**USD:13-41** 

well as `<control-D>` . 

## **line** 

A single-line address, given in any of the forms described in the section entitled “ **Ex Addressing** ”. The default for `line` is the current line. 

## **range** 

A line, or a pair of line addresses, separated by a comma or semicolon. (See the section entitled “ **Ex Addressing** ” for more information.) The default for range is the current line _only_ , i.e. “ `.,.` ”. A percent sign (“ `%` ”) stands for the range “ `1,$` ”. The starting address must be less than, or equal to, the ending address. 

## **count** 

A positive integer, specifying the number of lines to be affected by the command; the default is 1. Generally, a count past the end-of-file may be specified, e.g. the command “ `p 3000` ” in a 10 line file is acceptable, and will print from the current line through the last line in the file. 

## 

One or more of the characters “#”, “p”, and “l”. completes, the addressed line(s) are written out as if by the corresponding **#** , **l** or **p** commands. In addition, any number of “ `+` ” or “ `−` ” characters can be specified before, after, or during the flags, in which case the line written is not necessarily the one affected by the command, but rather the line addressed by the offset address specified. The default for `flags` is none. 

## 

File names are subjected to normal _sh_ (1) word expansions. 

`/tmp` ”. This will be replaced with a temporary file name which can be used for temporary work, e.g. “ `:e /tmp` ” 

command is the _last_ line addressed by the range, and `count` - subsequent lines are affected by the command, e.g. the command “ `2,3p4` ” prints out lines 3, 4, 5 and 6. 

**list** , **number** or **print** command. The command used is the most recent of the three commands to have been used (including any use as a flag). If none of these commands have been used before, the **print** command is the implied command. When no range or count is specified and the command line is a blank line, the current line is incremented by 1 and then the current line is displayed. 

mand name. Any object following a command name (such as buffer, file, etc.), that begins with an alphabetic character, should be separated from the command name by at least one whitespace character. 

Any character, including `<carriage-return>` , “ `%` ” and “ `#` ” retain their literal value when preceded by a backslash. 

## **17. Ex Commands** 

The following section describes the commands available in the **ex** editor. In each entry below, the tag line is a usage synopsis for the command. 

Each command can be entered as the abbreviation (those characters in the synopsis command word preceding the “[” character), the full command (all characters shown for the command word, omitting the “[” and “]” characters), or any leading subset of the full command down to the 


**USD:13-42** 

abbreviation. For example, the args command (shown as “ `ar[gs]` ” in the synopsis) can be entered as “ `ar` ”, “ `arg` ” or “ `args` ”. 

Each **ex** command described below notes the new current line after it is executed, as well as any options that affect the command. 

## **"** 

A comment. Command lines beginning with the double-quote character (“ `"` ”) are ignored. This permits comments in editor scripts and startup files. 

## **<control-D>** 

## 

Scroll the screen. Write the next N lines, where N is the value of the **scroll** option. The command is the end-of-file terminal character, which may be different on different terminals. Traditionally, it is the `<control-D>` key. 

Historically, the **eof** command ignored any preceding count, and the `<end-of-file>` character was ignored unless it was entered as the first character of the command. This implementation treats it as a command _only_ if entered as the first character of the command line, and otherwise treats it as any other character. 

Line: Set to the last line written. Options: Affected by the **scroll** option. 

## **! argument(s)** 

## **[range]! argument(s)** 

remainder of the line after the “ `!` ” character is passed to the program named by the **shell** option, as a single argument. 

Within the rest of the line, “ `%` ” and “ `#` ” are expanded into the current and alternate pathnames, respectively. The character “ `!` ” is expanded with the command text of the previous **!** command. (Therefore, the command **!!** repeats the previous **!** command.) The special meanings of “ `%` ”, “ `#` ”, and “ `!` ” can be overridden by escaping them with a backslash. If no **!** or **:!** command has yet been executed, it is an error to use an unescaped “ `!` ” character. The **!** command does _not_ do shell expansion on the strings provided as arguments. If any of the above expansions change the command the user entered, the command is redisplayed at the bottom of the screen. 

**Ex** then executes the program named by the **shell** option, with a **−c** ments (which are bundled into a single argument). 

The **!** 

A single “ `!` ” character is displayed when the command completes. 

In the second form of the **!** command, the remainder of the line after the “ `!` ” is passed to the program named by the **shell** option, as described above. The specified lines are passed to the program as standard input, and the standard and standard error output of the program replace the original lines. 


**USD:13-43** 

Line: Options: Affected by the **shell** and **warn** options. 

## 

## 

Display the selected lines, each preceded with its line number. 

The line number format is “%6d”, followed by two spaces. 

Line: Set to the last line displayed. Options: Affected by the **list** option. 

## **@ buffer** 

## *** buffer** 

Execute a buffer. Each line in the named buffer is executed as an **ex** command. If no buffer is specified, or if the specified buffer is “ `@` ” or “ `*` ”, the last buffer executed is used. 

## 

Shift lines left or right. **<** command) or right (for the **>** command), by the number of columns specified by the **shiftwidth** option. Only leading whitespace characters are deleted when shifting left; once the first column of the line contains a nonblank character, the **shift** command will succeed, but the line will not be modi- 

If the command character **<** or **>** is repeated more than once, the command is repeated once for each additional command character. 

Line: If the current line is set to one of the lines that are affected by the command, it is unchanged. Otherwise, it is set to the first nonblank character of the lowest numbered line shifted. Options: Affected by the **shiftwidth** option. 

## 

Display the line number of `line` 

Line: Unchanged. Options: None. 

## 

Shift right. the **shiftwidth** option, by inserting tab and space characters. Empty lines are not changed. 

If the command character “ `>` ” is repeated more than once, the command is repeated once for each additional command character. 

Line: Options: Affected by the **shiftwidth** option. 

## **ab[brev] lhs rhs** 

Add an abbreviation to the current abbreviation list. When inserting text in **vi** , each time a non-word character is entered after a word character, a set of characters ending at the word character are checked for a match with `lhs` . If a match is found, they are replaced with `rhs` . The set of characters that are checked for a match are defined as follows, for inexplicable 


**USD:13-44** 

historical reasons. If only one or two characters were entered before the non-word character that triggered the check, and after the beginning of the insertion, or the beginning of the line or the file, or the last `<blank>` character that was entered, then the one or the both characters are checked for a match. Otherwise, the set includes both characters, as well as the characters that precede them that are the same word class (i.e. word or non-word) as the **second** to last character entered before the non-word character that triggered the check, back to the first `<blank>` character, the beginning of the insertion, or the beginning of the line or the file. 

For example, the abbreviations: 

`:abbreviate abc ABC :abbreviate #i #include :abbreviate /*#i /*#include` will all work, while the abbreviations: 

`:abbreviate a#i A#include :abbreviate /* /********************` will not work, and are not permitted by **nvi** . 

To keep the abbreviation expansion from happening, the character immediately following the `lhs` characters should be quoted with a `<literal-next>` character. 

The replacement `rhs` is itself subject to both further abbreviation expansion and further map expansion. 

Line: Unchanged. Options: None. 

## **[line] a[ppend][!]** 

beginning of the file. Set to the last line input. If no lines are input, then set to `line` , or to the first line of the file if a `line` of 0 was specified. Following the command name with a “ `!` ” character causes the **autoindent** option to be toggled for the duration of the command. 

Line: Unchanged. Options: Affected by the **autoindent** and **number** options. 

## **ar[gs]** 

Display the argument list. The current argument is displayed inside of “ `[` ” and “ `]` ” characters. The argument list is the list of operands specified on startup, which can be replaced using the **next** command. 

Line: Unchanged. Options: None. 

## **bg** 

**Vi** mode only. Background the current screen. The screen is unchanged, but is no longer accessible and disappears from the display. Use the **fg** command to bring the screen back to the display foreground. 


**USD:13-45** 

Line: Set to the current line when the screen was last edited. Options: None. 

## **[range] c[hange][!] [count]** 

Replace the lines with input text. Following the command name with a “ `!` ” character causes the **autoindent** option to be toggled for the duration of the command. 

Line: Set to the last line input, or, if no lines were input, set to the line before the target line, or to the first line of the file if there are no lines preceding the target line. Options: Affected by the **autoindent** and **number** options. 

## **chd[ir][!] [directory]** 

## **cd[!] [directory]** 

Change the current working directory. The `directory` argument is subjected to _sh_ (1) word expansions. When invoked with no directory argument and the `HOME` environment variable is set, the directory named by the `HOME` environment variable becomes the new current directory. Otherwise, the new current directory becomes the directory returned by the _getpwent_ (3) routine. 

The **chdir** file. You can override this check by appending a “ `!` ” character to the command. 

Line: Unchanged. Options: Affected by the **cdpath** option. 

## 

## 

lines at the beginning of the file. 

Line: Unchanged. Options: None. 

## **cs[cope] command [args]** 

Execute a **cscope** command. For more information, see the section of the reference manual entitled “ **Tags, Tag Stacks, and Cscope** ”. 

## 

specified, in the unnamed buffer. If the command name is followed by a letter that could be interpreted as either a buffer name or a flag value (because neither a `count` or `flags` values were given), **ex** treats the letter as a `flags` value if the letter immediately follows the command name, without any whitespace separation. If the letter is preceded by whitespace characters, it treats it as a buffer name. 

Line: Set to the line following the deleted lines, or to the last line if the deleted lines were at the end. Options: None. 

## **di[splay] b[uffers] | c[onnections] | s[creens] | t[ags]** 

Display buffers, **cscope** connections, screens or tags. The **display** command takes one of three additional arguments, which are as follows: 


**USD:13-46** 

b[uffers] Display all buffers (including named, unnamed, and numeric) that contain text. c[onnections] 

Display the source directories for all attached **cscope** databases. s[creens] Display the file names of all background screens. t[ags] Display the tags stack. 

Line: Unchanged. Options: None. 

## 

## 

command will fail. You can override this by appending a “ `!` ” character to the command name. 

If the “ `+cmd` ” **ex** command will be executed in the new **ex** command may be used, although the most common use of this feature is to specify a line number or search pattern to set the initial location in the new file. 

**Edit** or **Ex** , while in **vi** in a new screen. In this case, any modifications to the current file are ignored. 

Line: If tion in the file. If that position does not exist, or you have not previously edited the file, the current line will be set to the first line of the file if you are in **vi** mode, and the last line of the file if you are in **ex** . 

Options: None. 

## **exu[sage] [command]** 

Display usage for an **ex** command. If `command` mand is displayed. Otherwise, usage statements for all **ex** commands are displayed. 

Line: Unchanged. Options: None. 

## 

is changed to the specified name. The current pathname, the number of lines, and the current position in the file are displayed. 

Line: Unchanged. Options: None. 

## **fg [name]** 

**Vi** mode only. If the argument name doesn’t exactly match the name of a file displayed by a background screen, it is compared against the last component of each of the file names. If no background screen is specified, the first background screen is foregrounded. 

By default, foregrounding causes the current screen to be swapped with the backgrounded screen. Capitalizing the first letter of the command, i.e. **Fg** , will foreground the backgrounded screen in a new screen instead of swapping it with the current screen. 


**USD:13-47** 

Line: Set to the current line when the screen was last edited. Options: None. 

## **[range] g[lobal] /pattern/ [commands]** 

## **[range] v /pattern/ [commands]** 

Apply commands to lines matching (or not matching) a pattern. The lines within the given range that match (“ `g[lobal]` ”), or do not match (“ `v` ”) the given pattern are selected. Then, the specified **ex** command(s) are executed with the current line (“ `.` ”) set to each selected line. If no range is specified, the entire file is searched for matching, or not matching, lines. 

`<newline>` character with a backslash, or by separating commands with a “ `|` ” character. If no commands are specified, the command defaults to the **print** command. 

For the **append** , **change** and **insert** commands, the input text must be part of the global command line. In this case, the terminating period can be omitted if it ends the commands. 

The **visual ex** commands. In this mode, input is taken from the terminal. Entering a **Q** command in **vi** mode causes the next line matching the pattern to be selected and **vi** to be reentered, until the list is exhausted. 

The **global** , **v** and **undo** commands cannot be used as part of these commands. 

The editor options **autoindent** , **autoprint** and **report** are turned off for the duration of the **global** and **v** commands. 

Line: Options: Affected by the **ignorecase** and **magic** options. Turns off the **autoindent** , **autoprint** and **report** options. 

## **he[lp]** 

Display a help message. 

Line: Unchanged. Options: None. 

## **[line] i[nsert][!]** 

Following the command name with a “ `!` ” character causes the **autoindent** option setting to be toggled for the duration of this command. 

Line: Set to the last line input; if no lines were input, set to the line before the target line, or to the first line of the file if there are no lines preceding the target line. Affected by the **autoindent** and **number** options. 

## 

Join lines of text together. 

A `count range` plus `count` subsequent lines will be joined. (Note, this differs by one from the general rule where only `count` - subsequent lines are affected.) 

If the current line ends with a whitespace character, all whitespace is stripped from the next line. Otherwise, if the next line starts with a open parenthesis (“ `(` ”), do nothing. Otherwise, if 


**USD:13-48** 

the current line ends with a question mark (“ `?` ”), period (“ `.` ”) or exclamation point (“ `!` ”), insert two spaces. Otherwise, insert a single space. 

Appending a “ `!` ” character to the command name causes a simpler join with no white-space processing. 

Line: Unchanged. Options: None. 

## 

Display the lines unambiguously. Tabs are displayed as “ `ˆI` ”, and the end of the line is marked with a “ `$` ” character. 

Line: Set to the last line displayed. Options: Affected by the **number** option. 

## **map[!] [lhs rhs]** 

**vi** only). 

If “ `lhs` ” and “ `rhs` ” If a “ `!` ” character is appended to to the command, the text input mode maps are displayed. 

Otherwise, when the “ `lhs` ” character sequence is entered in **vi** , the action is as if the corresponding “ `rhs` ” had been entered. If a “ `!` ” character is appended to the command name, the mapping is effective during text input mode, otherwise, it is effective during command mode. This allows “ `lhs` ” to hav e two different macro definitions at the same time: one for command mode and one for input mode. 

Whitespace characters require escaping with a `<literal-next>` character to be entered in the `lhs` string in visual mode. 

Normally, keys in the `rhs` string are remapped (see the **remap** option), and it is possible to create infinite loops. However, keys which map to themselves are not further remapped, regardless of the setting of the **remap** option. For example, the command “ `:map n nz.` ” maps the “ `n` ” key to the **n** and **z** commands. 

`<interrupt>` character. 

Line: Unchanged. Options: Affected by the **remap** option. 

## **[line] ma[rk] <character>** 

## **[line] k <character>** 

Mark the line with the mark `<character>` . The expressions “ `’<character>` ” and “ `‘<character>` ” can then be used as an address in any command that uses one. 

Line: Unchanged. Options: None. 

## **[range] m[ove] line** 

A target line of 0 places the lines at the beginning 


**USD:13-49** 

Line: Options: None. 

## 

Information is written in a form which can later be read back in using the **ex source** command. If `file` already exists, the **mkexrc** command will fail. This check can be overridden by appending a “ `!` ” character to the command. 

Line: Unchanged. Options: None. 

## 

The **next** fied since the last complete write. This check can be overridden by appending the “ `!` ” character to the command name. The argument list can optionally be replaced by specifying a new one as arguments to this command. In this case, editing starts with the first file on the new list. 

**Next** , while in **vi** mode, will set the argument list and edit the file in a new screen. In this case, any modifications to the current file are ignored. 

Line: Set as described for the **edit** command. Options: Affected by the options **autowrite** and **writeany** . 

## 

Enter open mode. Open mode is the same as being in **vi** , but with a one-line window. All the standard **vi** commands are available. If a match is found for the optional RE argument, the cursor is set to the start of the matching pattern. 

_This command is not yet implemented._ 

Line: Unchanged, where the matching pattern is found. 

Options: Affected by the **open** option. 

## **pre[serve]** 

**ex −r** served, an email message is sent to the user. 

Line: Unchanged. Options: None. 

## **prev[ious][!]** 

The **previous** been modified since the last complete write. This check can be overridden by appending the “ `!` ” character to the command name. 

in a new screen. In this case, any modifications to the current file are ignored. **Previous** , while in **vi** 

Line: Set as described for the **edit** command. 


**USD:13-50** 

Options: Affected by the options **autowrite** and **writeany** . None. 

## 

Line: Set to the last line displayed. Options: Affected by the **list** and **number** option. 

## **[line] pu[t] [buffer]** 

Append buffer contents to the current line. the line, otherwise, the contents of the unnamed buffer are used. 

Line: Set to the line after the current line. Options: None. 

## **q[uit][!]** 

End the editing session. **quit** command will fail. This check may be overridden by appending a “ `!` ” character to the command. 

**quit** command will fail. Appending a “ `!` ” character to the command name or entering two **quit** commands (i.e. **wq** , **quit** , **xit** or **ZZ** ) in a row) will override this check and the editor will exit. 

Line: Unchanged. Options: None. 

## 

A copy of If `line` is 0, the copy is inserted at the beginning of the file. If no file is specified, the current file is read; if there is no current file, then `file` becomes the current file. If there is no current file and no `file` is specified, then the **read** command will fail. 

If `file` is preceded by a “ `!` ” character, `file` is treated as if it were a shell command, and passed to the program named by the **shell** edit option. The standard and standard error outputs of that command are read into the file after the specified line. The special meaning of the “ `!` ” character can be overridden by escaping it with a backslash (“ `\` ”) character. 

Line: When executed from **ex** , the current line is set to the last line read. When executed from **vi** , the current line is set to the first line read. Options: None. 

## 

Recover `file` if it was previously saved. If no saved **recover** command behaves equivalently to the **edit** command. 

Line: Set as described for the **edit** command. Options: None. 

## **res[ize] [+|-]size** 

**Vi** mode only. Grow or shrink the current screen. If `size` is a positive, signed number, the current screen is grown by that many lines. If `size` is a negative, signed number, the current screen is shrunk by that many lines. If `size` is not signed, the current screen is set to the 


**USD:13-51** 

`size` . Applicable only to split screens. 

Line: Unchanged. Options: None. 

## **rew[ind][!]** 

Rewind the argument list. the **rewind** command will fail. This check may be overridden by appending the “ `!` ” character to the command. 

Line: Set as described for the **edit** command. Options: Affected by the **autowrite** and **writeany** options. 

## **se[t] [option[=[value]] ...] [nooption ...] [option? ...] [all]** 

Display or set editor options. **term** , and any editor options whose values have been changed from the default settings are displayed. If the argument `all` is specified, the values of all of editor options are displayed. 

Specifying an option name followed by the character “ `?` ” causes the current value of that option to be displayed. The “ `?` ” can be separated from the option name by whitespace characters. The “ `?` ” is necessary only for Boolean valued options. Boolean options can be given values by the form “ `set option` ” to turn them on, or “ `set nooption` ” to turn them off. String and numeric options can be assigned by the form “ `set option=value` ”. Any whitespace characters in strings can be included literally by preceding each with a backslash. More than one option can be set or listed by a single set command, by specifying multiple arguments, each separated from the next by whitespace characters. 

Line: Unchanged. Options: None. 

## **sh[ell]** 

Run the shell program. The program named by the **shell** option is run with a **−i** (for interactive) flag. Editing is resumed when that program exits. 

Line: Unchanged. Options: Affected by the **shell** option. 

## 

Read and execute **ex Source** commands may be nested. 

Line: Unchanged. Options: None. 

## 

## **[range] ˜ [options] [count] [flags]** 

Make substitutions. Replace `pattern` with the string `replace` on the specified line(s). If the “ `/pattern/repl/` ” argument is not specified, the “ `/pattern/repl/` ” from the previous **substitute** command is used. Any character other than an alphabetic, numeric, <blank> or backslash character may be used as the delimiter. 


**USD:13-52** 

If `options` includes the letter “ `c` ” each replacement is done. An affirmative response (in English, a “ `y` ” character) causes the replacement to be made. A quit response (in English, a “ `q` ” character) causes the **substitute** command to be terminated. Any other response causes the replacement not to be made, and the **substitute** command continues. If `options` includes the letter “ `g` ” (global), all nonoverlapping instances of `pattern` in the line are replaced. 

The **&** version of the command is the same as not specifying a pattern or replacement string to the **substitute** command, and the “ `&` ” is replaced by the pattern and replacement information from the previous substitute command. 

The **˜** version of the command is the same as **&** and **s** , except that the search pattern used is the last RE used in _any_ command, not necessarily the one used in the last **substitute** command. 

For example, in the sequence 

```
s/red/blue/
/green
˜
```

the “ `˜` ” is equivalent to “ `s/green/blue/` ”. 

The **substitute** command may be interrupted, using the terminal interrupt character. All substitutions completed before the interrupt are retained. 

Line: Set to the last line upon which a substitution was made. Options: Affected by the **ignorecase** and **magic** option. 

## **su[spend][!] st[op][!] <control-Z>** 

Suspend the edit session. Appending a “ `!` ” character to these commands turns off the **autowrite** option for the command. 

Line: Unchanged. Options: Affected by the **autowrite** and **writeany** options. 

## **ta[g][!] tagstring** 

edited. If the current file has been modified since the last complete write, the **tag** command will fail. This check can be overridden by appending the “ `!` ” character to the command name. 

The **tag** command searches for `tagstring` (See _ctags_ (1) for more information on tags files.) 

**Tag** , while in **vi** new screen. In this case, any modifications to the current file are ignored. 

Line: Set to the line indicated by the tag. Options: Affected by the **autowrite** , **taglength** , **tags** and **writeany** options. 

## **tagn[ext][!]** 

then the new file is edited. If the current file has been modified since the last complete write, 


**USD:13-53** 

the **tagnext** command will fail. This check can be overridden by appending the “ `!` ” character to the command name. 

**Tagnext** , while in **vi** a new screen. In this case, any modifications to the current file are ignored. 

Line: Set to the line indicated by the tag. Options: Affected by the **autowrite** and **writeany** options. 

## 

If neither `file` or `number` **tagpop** command pops to the most recent entry on the tags stack. If `file` or `number` is specified, the **tagpop** command pops to the most recent entry in the tags stack for that file, or numbered entry in the tags stack, respectively. (See the **display** command for information on displaying the tags stack.) 

**tagpop** command will fail. This check may be overridden by appending a “ `!` ” character to the command name. 

Line: Set to the line indicated by the tag. Options: Affected by the **autowrite** and **writeany** options. 

## **tagp[rev][!]** 

If the context is in a different file, then the new file is edited. If the current file has been modified since the last complete write, the **tagprev** command will fail. This check can be overridden by appending the “ `!` ” character to the command name. 

**Tagprev** , while in **vi** a new screen. In this case, any modifications to the current file are ignored. 

Line: Set to the line indicated by the tag. Options: Affected by the **autowrite** and **writeany** options. 

## **tagt[op][!]** 

Pop to the least recent tag on the tags stack, clearing the tags stack. 

**tagtop** command will fail. This check may be overridden by appending a “ `!` ” character to the command name. 

Line: Set to the line indicated by the tag. Options: Affected by the **autowrite** and **writeany** options. 

## **una[bbrev] lhs** 

Delete an abbreviation. Delete `lhs` from the current list of abbreviations. 

Line: Unchanged. Options: None. 

## **u[ndo]** 

Changes made by **global** , **v** , **visual** and map sequences are considered a single command. If repeated, the **u** command alternates between these two states, and is its own inverse. 


**USD:13-54** 

Line: Options: None. 

## **unm[ap][!] lhs** 

Unmap a mapped string. `lhs` . If a “ `!` ” character is appended to the command name, delete the text input mode map definition instead. 

Line: Unchanged. Options: None. 

## **ve[rsion]** 

Display the version of the **ex/vi** editor. 

## 

**Ex** mode only. Enter **vi** . The `type` is optional, and can be “ `−` ”, “ `+` ” or “ `ˆ` ”, as in the **ex z** command, to specify the position of the specified line in the screen window. (The default is to place the line at the top of the screen window.) A `count` specifies the number of lines that will initially be displayed. (The default is the value of the **window** editor option.) 

Line: Unchanged unless `line` Options: None. 

## 

**Vi** mode only. `edit[!] [+cmd] [file]` ” command. 

this case, any modifications to the current file are ignored. **Visual** , screen. In 

## **viu[sage] [command]** 

Display usage for a **vi** command. If `command` mand is displayed. Otherwise, usage statements for all **vi** commands are displayed. 

Line: Unchanged. Options: None. 

## 

## 

## 

## 

`file` . If `file` is not specified, the current pathname is used. If `file` is specified, and it exists, or if the current pathname was set using the **file** command, and the file already exists, these commands will fail. Appending a “ `!` ” character to the command name will override this check and the write will be attempted, regardless. 

Specifying the optional “ `>>` ” case no tests are made for the file already existing. 

`!` ” character, the program named by the shell edit option is invoked with file as its second argument, and the specified lines are passed as standard input to that command. The “ `!` ” in this usage must be separated from command name by at least one whitespace character. The special meaning of the “ `!` ” may be overridden by escaping it with a backslash (“ `\` ”) character. 


**USD:13-55** 

The **wq** further files to edit. Appending a “ `!` ” character to the command name or entering two “quit” commands (i.e. **wq** , **quit** , **xit** or **ZZ** ) in a row) will override this check and the editor will exit, ignoring any files that have not yet been edited. 

The **wn** version of the write command will move to the write fails. 

Line: Unchanged. Options: Affected by the **readonly** and **writeany** options. 

## 

`file` , if been modified since the last complete write to any file. If no `range` is specified, the entire file is written. 

The **xit** Appending a “ `!` ” character to the command name or entering two “quit” commands (i.e. **wq** , **quit** , **xit** or **ZZ** ) in a row) will override this check and the editor will exit, ignoring any files that have not yet been edited. 

Line: Unchanged. Options: Affected by the **readonly** and **writeany** options. 

## **[range] ya[nk] [buffer] [count]** 

Line: Unchanged. Options: None. 

## 

Adjust the window. If no `type count` displayed. The default `count` is the value of the **window** option. The `type` argument changes the position at which `line` is displayed on the screen by changing the number of lines displayed before and after `line` . The following `type` characters may be used: 

- Place the line at the bottom of the screen. 

- + Place the line at the top of the screen. 

- . Place the line in the middle of the screen. 

- ˆ Write out count lines starting `count * 2` lines before `line` ; the net effect of this is that a “ `zˆ` ” command following a **z** command writes the previous page. 

- = Center `line` on the screen with a line of hyphens displayed immediately before and after it. The number of preceding and following lines of text displayed are reduced to account for those lines. 

Line: Set to the last line displayed, with the exception of the `type` , where the current line is set to the line specified by the command. Options: Affected by the **scroll** option. 

## **18. Set Options** 

There are a large number of options that may be set (or unset) to change the editor’s behavior. This section describes the options, their abbreviations and their default values. 


**USD:13-56** 

equivalent abbreviations. (Regardless of the abbreviations, it is only necessary to use the minimum number of characters necessary to distinguish an abbreviation from all other commands for it to be accepted, in **nex** / **nvi** . Historically, only the full name and the official abbreviations were accepted by **ex** / **vi** . Using full names in your startup files and environmental variables will probably make them more portable.) The part in square brackets is the default value of the option. Most of the options are boolean, i.e. they are either on or off, and do not have an associated value. 

Options apply to both **ex** and **vi** 

With a few exceptions, all options are settable per screen, i.e. the **tags** option can be set differently in each screen. The exceptions are the **columns** , **lines** , **secure** and **term** options. Changing these options modifies the respective information for all screens. 

For information on modifying the options or to display the options and their current values, see the “set” command in the section entitled “ **Ex Commands** ”. 

## **altwerase [off]** 

**Vi** only. Change how **vi** does word erase during text input. When this option is set, text is broken up into three classes: alphabetic, numeric and underscore characters, other nonblank characters, and blank characters. Changing from one class to another marks the end of a word. In addition, the class of the first character erased is ignored (which is exactly what you want when erasing pathname components). 

## **autoindent, ai [off]** 

If this option is set, whenever you create a new line (using the **vi A** , **a** , **C** , **c** , **I** , **i** , **O** , **o** , **R** , **r** , **S** , and **s** commands, or the **ex append** , **change** , and **insert** commands) the new line is automatically indented to align the cursor with the first nonblank character of the line from which you created it. Lines are indented using tab characters to the extent possible (based on the value of the **tabstop** option) and then using space characters as necessary. For commands inserting text into the middle of a line, any blank characters to the right of the cursor are discarded, and the first nonblank character to the right of the cursor is aligned as described above. 

The indent characters are themselves somewhat special. If you do not enter more characters on the new line before moving to another line, or entering `<escape>` , the indent character will be deleted and the line will be empty. For example, if you enter `<carriage-return>` twice in succession, the line created by the first `<carriage-return>` will not have any characters in it, regardless of the indentation of the previous or subsequent line. 

Indent characters also require that you enter additional erase characters to delete them. For example, if you have an indented line, containing only blanks, the first `<word-erase>` character you enter will erase up to end of the indent characters, and the second will erase back to the beginning of the line. (Historically, only the `<control-D>` key would erase the indent characters. Both the `<control-D>` key and the usual erase keys work in **nvi** .) In addition, if the cursor is positioned at the end of the indent characters, the keys “ `0<control-D>` ” will erase all of the indent characters for the current line, resetting the indentation level to 0. Similarly, the keys “ `ˆ<control-D>` ” will erase all of the indent characters for the current line, leaving the indentation level for future created lines unaffected. 

Finally, if the **autoindent** option is set, the **S** and **cc** of the line to the end of the line, instead of from the beginning of the line to the end of the line. 

## **autoprint, ap [off]** 

**Ex** only. Cause the current line to be automatically displayed after the **ex** commands **<** , **>** , **copy** , **delete** , **join** , **move** , **put** , **t** , **Undo** , and **undo** . This automatic display is suppressed during 


**USD:13-57** 

**global** and **v** play the line. 

## **autowrite, aw [off]** 

If this option is set, the **vi !** , **ˆˆ** , **ˆ]** and **<control-Z>** commands, and the **ex edit** , **next** , **rewind** , **stop** , **suspend** , **tag** , **tagpop** , and **tagtop** commands automatically write the current file back to the current file name if it has been modified since it was last written. If the write fails, the command fails and goes no further. 

`!` ” to the **ex** commands **next** , **rewind** , **stop** , **suspend** , **tag** , **tagpop** , and **tagtop** stops the automatic write from being attempted. 

(Historically, the **next** Note, the **ex** commands **edit** , **quit** , **shell** , and **xit** are _not_ affected by the **autowrite** option. 

The **autowrite** 

## **backup [""]** 

a ten, the file’s current contents are copied to it. The pathname is “ `#` ”, “ `%` ” and “ `!` ” expanded. 

`N` ”, a version number is appended to the pathname (and the “ `N` ” character is then discarded). Version numbers are always incremented, and each backup file will have a version number one greater than the highest version number currently found in the directory. 

not accessible by any other user. 

## **beautify, bf [off]** 

If this option is set, all control characters that are not currently being specially interpreted, other than `<tab>` , `<newline>` , and `<form-feed>` , are discarded from commands read in by **ex** from command files, and from input text entered to **vi** (either into the file or to the colon command line). Te xt files read by **ex** / **vi** are _not_ affected by the **beautify** option. 

## **cdpath [environment variable CDPATH, or current directory]** 

This option is used to specify a colon separated list of directories which are used as path prefixes for any relative path names used as arguments for the **cd** command. The value of this option defaults to the value of the environmental variable `CDPATH` if it is set, otherwise to the current directory. For compatibility with the POSIX 1003.2 shell, the **cd** command does _not_ check the current directory as a path prefix for relative path names unless it is explicitly specified. It may be so specified by entering an empty string or a “ `.` ” character into the `CDPATH` variable or the option value. 

## **cedit [no default]** 

This option adds the ability to edit the colon command-line history. This option is set to a string. Whenever the first character of that string is entered on the colon command line, you will enter a normal editing window on the collected commands that you’ve entered on the **vi** colon command-line. You may then modify and/or execute the commands. All normal text editing is available, except that you cannot use **<control-W>** to switch to an alternate screen. Entering a **<carriage-return>** will execute the current line of the screen window as an ex command in the context of the screen from which you created the colon command-line screen, and you will then return to that screen. 


**USD:13-58** 

Because of **vi** the `<escape>` character. To set it to `<escape>` , use “ `set cedit=<literalnext><escape>` ”. 

If the **cedit** edit option is set to the same character as the edit option, **vi** will perform colon command-line editing if the character is entered as the first character of the line, otherwise, **vi** will perform file name expansion. 

## **columns, co [80]** 

The number of columns in the screen. Setting this option causes **ex** / **vi** to set (or reset) the environmental variable `COLUMNS` . See the section entitled “ **Sizing the Screen** ” more information. 

## **comment [off]** 

**Vi** only. If `#` ”, “ `/*` ” or “ `//` ”, this option causes **vi** to skip to the end of that shell, C or C++ comment (probably a terribly boring legal notice) before displaying the file. 

## **directory, dir [environment variable TMPDIR, or /tmp]** 

The environmental variable `TMPDIR` is used as the default value if it exists, otherwise `/tmp` is used. 

## **edcompatible, ed [off]** 

**substitute** commands, instead of initializing them as unset for each new command. Specifying pattern and replacement strings to the **substitute** command unsets the “c” and “g” suffixes as well. 

## **escapetime [1]** 

The 10th’s of a second **ex** / **vi** waits for a subsequent key to complete an `<escape>` key mapping. 

## **errorbells, eb [off]** 

**Ex** only. **Ex** error messages are normally presented in inverse video. If that is not possible for the terminal, setting this option causes error messages to be announced by ringing the terminal bell. 

## **exrc, ex [off]** 

If this option is turned on in the EXINIT environment variables, or the system or $HOME startup files, the local startup files are read, unless they are the same as the system or $HOME startup files or fail to pass the standard permission checks. See the section entitled “ **Startup Information** ” for more information. 

## **extended [off]** 

This option causes all regular expressions to be treated as POSIX 1003.2 Extended Regular Expressions (which are similar to historic _egrep_ (1) style expressions). 

## 

This option adds the ability to do shell expansion when entering input on the colon command line. This option is set to a string. Whenever the first character of that string is entered on the colon command line, the <blank> delimited string immediately before the cursor is expanded as if it were followed by a `*` character, and file name expansion for the **ex** edit command was done. If no match is found, the screen is flashed and text input resumed. If a single match results, that match replaces the expanded text. In addition, if the single match is for a directory, a `/` character is appended and file completion is repeated. If more than a single match 


**USD:13-59** 

results, any displayed, and text input resumed. 

Because of **vi** command values, `<escape>` and `<tab>` . To set it to `<escape>` , use “ `set filec=<literal-next><escape>` ”. To set it to `<tab>` , use “ `set filec=\<tab>` ”. 

If the **cedit** edit option is set to the same character as the edit option, **vi** will perform colon command-line editing if the character is entered as the first character of the line, otherwise, **vi** will perform file name expansion. 

## 

has the capability. 

## **hardtabs, ht [8]** 

the tab expansion done by the operating system and/or the terminal itself. As **nex** / **nvi** never writes `<tab>` characters to the terminal, unlike historic versions of **ex** / **vi** , this option does not currently have any affect. 

## **iclower [off]** 

The **iclower** edit option makes all Regular Expressions case-insensitive, as long as an uppercase letter does not appear in the search string. 

## **ignorecase, ic [off]** 

This option causes regular expressions, both in **ex** commands and in searches, to be evaluated in a case-insensitive manner. 

## **keytime [6]** 

The 10th’s of a second **ex** / **vi** waits for a subsequent key to complete a key mapping. 

## **leftright [off]** 

**Vi** only. This option causes the screen to be scrolled left-right to view lines longer than the screen, instead of the traditional **vi** screen interface which folds long lines at the right-hand margin of the terminal. 

## **lines, li [24]** 

**Vi** only. The number of lines in the screen. Setting this option causes **ex** / **vi** to set (or reset) the environmental variable `LINES` . See the section entitled “ **Sizing the Screen** ” for more information. 

## **lisp [off]** 

**Vi** only. This option changes the behavior of the **vi (** , **)** , **{** , **}** , **[[** and **]]** commands to match the Lisp language. Also, the **autoindent** option’s behavior is changed to be appropriate for Lisp. 

_This option is not yet implemented._ 

## **list [off]** 

played as control characters, i.e. “ `ˆI` ”, and the ends of lines are marked with a “ `$` ” character. 


**USD:13-60** 

## **lock [on]** 

This option causes the editor to attempt to get an exclusive lock on any or written. Reading or writing a file that cannot be locked produces a warning message, but no other effect. Editing a file that cannot be locked results in a read only edit session, as if the **readonly** edit option were set. 

## **magic [on]** 

This option is on by default. Turning the **magic** option off causes all regular expression characters except for “ `ˆ` ” and “ `$` ”, to be treated as ordinary characters. To re-enable characters individually, when the **magic** option is off, precede them with a backslash “ `\` ” character. See the section entitled “ **Regular Expressions and Replacement Strings** ” for more information. 

## **matchtime [7]** 

**Vi** only. The 10th’s of a second **vi** pauses on the matching character when the **showmatch** option is set. 

## **mesg [on]** 

This option allows other users to contact you using the _talk_ (1) and _write_ (1) utilities, while you are editing. **Ex** / **vi** does not turn message on, i.e. if messages were turned off when the editor was inv oked, they will stay turned off. This option only permits you to disallow messages for the edit session. See the _mesg_ (1) utility for more information. 

## **msgcat [./]** 

This option selects a message catalog to be used to display error and informational messages in a specified language. If the value of this option ends with a ’/’, it is treated as the name of a directory that contains a message catalog “ `vi_XXXX` ”, where “ `XXXX` ” is the value of the `LANG` environmental variable, if it’s set, or the value of the `LC_MESSAGES` environmental variable if it’s not. If neither of those environmental variables are set, or if the option doesn’t end in a ’/’, the option is treated as the full path name of the message catalog to use. 

If any messages are missing from the catalog, the backup text (English) is used instead. 

`catalog/README` for additional information on building and installing message catalogs. 

## **modelines, modeline [off]** 

If the **modelines** option is set, **ex** / **vi** file as it is read for editing, looking for any **ex** commands that have been placed in those lines. After the startup information has been processed, and before the user starts editing the file, any 

Commands were recognized by the letters “e” or “v” followed by “x” or “i”, at the beginning of a line or following a tab or space character, and followed by a “:”, an **ex** command, and another “:”. 

This option is a security problem of immense proportions, and should not be used under any circumstances. 

_This option will never be implemented._ 

## **noprint [""]** 

Characters that are never handled as printable characters. By default, the C library function _isprint_ (3) is used to determine if a character is printable or not. This edit option overrides that 


**USD:13-61** 

decision. 

## **number, nu [off]** 

Precede each line displayed with its current line number. 

## **octal [off]** 

Display unknown characters as octal numbers (“ `\###` ”), instead of the default hexadecimal (“ `\x##` ”). 

## **open [on]** 

**Ex** only. If this option is not set, the **open** and **visual** commands are disallowed. 

## **optimize, opt [on]** 

**Vi** only. Throughput of text is expedited by setting the terminal not to do automatic carriage returns when printing more than one (logical) line of output, greatly speeding output on terminals without addressable cursors when text with leading white space is printed. 

_This option is not yet implemented._ 

## **paragraphs, para [IPLPPPQPP LIpplpipbp]** 

**Vi** only. **{** and **}** commands. The value of this option must be a character string consisting of zero or more character pairs. 

In the text to be edited, the character string `<newline>.<char-pair>` , (where `<charpair>` is one of the character pairs in the option’s value) defines a paragraph boundary. For example, if the option were set to `LaA<space>##` , then all of the following additional paragraph boundaries would be recognized: 

<newline>.La <newline>.A<space> <newline>.## 

## **path []** 

The path option can be used to specify a <colon>-separated list of paths, similar to the `PATH` environment variable in the shells. If this option is set, the name of the file to be edited is not an absolute pathname, the first component of the filename is not “ `.` ” or “ `..` ”, and the file to be edited doesn’t exist in the current directory, the elements of the **path** option are sequentially searched for a file of the specified name. If such a file is found, it is edited. 

## **print [""]** 

Characters that are always handled as printable characters. By default, the C library function _isprint_ (3) is used to determine if a character is printable or not. This edit option overrides that decision. 

## **prompt [on]** 

**Ex** only. This option causes **ex** to prompt for command input with a “ `:` ” character; when it is not set, no prompt is displayed. 

## **readonly, ro [off]** 

Setting this option is equivalent to using the **−R** command line option, or executing the **vi** program using the name **view** . 


**USD:13-62** 

The **readonly** edit option is not usually persistent, like other edit options. If the **−R** command line option is set, **vi** is executed as **view** , or the **readonly** edit option is explicitly set, all files edited in the screen will be marked readonly, and the force flag will be required to write them. However, if none of these conditions are true, or the **readonly** edit option is explicitly unset, then the **readonly** edit option will toggle based on the write permissions of the file currently being edited as of when it is loaded into the edit buffer. In other words, the **readonly** edit option will be set if the current file lacks write permissions, and will not be set if the user has write permissions for the file. 

## **recdir [/var/tmp/vi.recover]** 

If you change the value of **recdir** , be careful to choose a directory whose contents are not regularly deleted. Bad choices include directories in memory based filesystems, or `/tmp` , on most systems, as their contents are removed when the machine is rebooted. 

Public directories like `/usr/tmp` and `/var/tmp` are usually safe, although some sites periodically prune old files from them. There is no requirement that you use a public directory, e.g. a sub-directory of your home directory will work fine. 

Finally, if you change the value of **recdir** , you must modify the recovery script to operate in your chosen recovery area. 

See the section entitled “ **Recovery** ” for further information. 

## **redraw, re [off]** 

**Vi** only. The editor simulates (using great amounts of output), an intelligent terminal on a dumb terminal (e.g. during insertions in **vi** the characters to the right of the cursor are refreshed as each input character is typed). 

_This option is not yet implemented._ 

## **remap [on]** 

Otherwise, each key is only remapped up to one time. For example, if “ `A` ” is mapped to “ `B` ”, and “ `B` ” is mapped to “ `C` ”, The keystroke “ `A` ” will be mapped to “ `C` ” if the **remap** option is set, and to “ `B` ” if it is not set. 

## **report [5]** 

Set the threshold of the number of lines that need to be changed or yanked before a message will be displayed to the user. For everything but the yank command, the value is the largest value about which the editor is silent, i.e. by default, 6 lines must be deleted before the user is notified. However, if the number of lines yanked is greater than _or equal to_ the set value, it is reported to the user. 

## **ruler [off]** 

**Vi** only. Display a row/column ruler on the colon command line. 

## **scroll, scr [(environment variable LINES - 1) / 2]** 

Set the number of lines scrolled by the **ex <control-D>** and commands. 

Historically, the **ex z** scroll value; the POSIX 1003.2 standard specified the window size, which is a better choice. 


**USD:13-63** 

## **searchincr [off]** 

The **searchincr** edit option makes the search commands **/** and **?** incremental, i.e. the screen is updated and the cursor moves to the matching text as the search pattern is entered. If the search pattern is not found, the screen is beeped and the cursor remains on the colon-command line. Erasing characters from the search pattern backs the cursor up to the previous matching text. 

## **sections, sect [NHSHH HUnhsh]** 

**Vi** only. **[[** and **]]** commands. The **sections** option should be set to a character string consisting of zero or more character pairs. In the text to be edited, the character string `<newline>.<char-pair>` , (where `<char-pair>` is one of the character pairs in the option’s value), defines a section boundary in the same manner that **paragraphs** option boundaries are defined. 

## **secure [off]** 

The **secure** edit option turns off all access to external programs. This means that the versions of the **read** and **write** commands that filter text through other programs, the **vi !** and **<controlZ>** commands, the **ex !** , **script** , **shell** , **stop** and **suspend** commands and file name expansion will not be permitted. Once set, the **secure** edit option may not be unset. 

## **shell, sh [environment variable SHELL, or /bin/sh]** 

Select the shell used by the editor. the **vi !** shell escape command and by the **ex shell** command. This program is also used to resolve any shell meta-characters in **ex** commands. 

## **shellmeta [˜{[*?$‘’"\]** 

The set of characters that **ex** any of the specified characters are found in the file name arguments to the **ex** commands, the arguments are expanded using the program defined by the **shell** option. The default set of characters is a union of meta characters from the Version 7 and the Berkeley C shell. 

## **shiftwidth, sw [8]** 

Set the autoindent and shift command indentation width. This width is used by the **autoindent** option and by the **<** , **>** , and **shift** commands. 

## **showmatch, sm [off]** 

**Vi** only. This option causes **vi** , when a “ `}` ” or “ `)` ” is matching “ `{` ” or “ `(` ”. See the **matchtime** option for more information. 

## **showmode, smd [off]** 

**Vi** only. This option causes **vi** to display a string identifying the current editor mode on the colon command line. The string is preceded by an asterisk (‘‘*’’) if the file has been modified since it was last completely written, 

## **sidescroll [16]** 

**Vi** only. Sets the number of columns that are shifted to the left or right, when **vi** is doing leftright scrolling and the left or right margin is crossed. See the **leftright** option for more information. 

## **slowopen, slow [off]** 

This option affects the display algorithm used by **vi** , holding off display updating during input of new text to improve throughput when the terminal in use is slow and unintelligent. 


**USD:13-64** 

_This option is not yet implemented._ 

## **sourceany [off]** 

If this option is turned on, **vi** than the editor user. See the section entitled “ **Startup Information** ” for more information. This option is a security problem of immense proportions, and should not be used under any circumstances. 

_This option will never be implemented._ 

## **tabstop, ts [8]** 

This option sets tab widths for the editor display. 

## **taglength, tl [0]** 

name. Setting the value to 0 makes all of the characters in the tag name significant. 

## **tags, tag [tags /var/db/libc.tags /sys/kern/tags]** 

## **term, ttytype, tty [environment variable TERM]** 

Set the terminal type. Setting this option causes **ex** / **vi** to set (or reset) the environmental variable `TERM` . 

## **terse [off]** 

This option has historically made editor messages less verbose. It has no effect in this implementation. See the **verbose** option for more information. 

## **tildeop [off]** 

Modify the **˜** command to take an associated motion. 

## **timeout, to [on]** 

If this option is set, **ex** / **vi** complete a key mapping (see the **keytime** option). If the option is not set, the editor waits until enough keys are entered to resolve the ambiguity, reg ardless of how long it takes. 

## **ttywerase [off]** 

**Vi** only. This option changes how **vi** does word erase during text input. If this option is set, text is broken up into two classes, blank characters and nonblank characters. Changing from one class to another marks the end of a word. 

## **verbose [off]** 

**Vi** only. **Vi** historically bells the terminal for many obvious mistakes, e.g. trying to move past the left-hand margin, or past the end of the file. If this option is set, an error message is displayed for all errors. 

## **w300 [no default]** 

**Vi** only. Set the window size if the baud rate is less than 1200 baud. See the **window** option for more information. 

## **w1200 [no default]** 

**Vi** only. Set the window size if the baud rate is equal to 1200 baud. See the **window** option for more information. 


**USD:13-65** 

## **w9600 [no default]** 

**Vi** only. Set the window size if the baud rate is greater than 1200 baud. See the **window** option for more information. 

## **warn [on]** 

**Ex** only. since it was last written, before a **!** command. 

## **window, w, wi [environment variable LINES - 1]** 

This option determines the default number of lines in a screenful, as displayed by the **z** command. It also determines the number of lines scrolled by the **vi** commands **<control-B>** and **<control-F>** , and the default number of lines scrolled by the **vi** commands **<control-D>** and **<control-U>** . The value of window can be unrelated to the real screen size, although it starts out as the number of lines on the screen. See the section entitled “ **Sizing the Screen** ” for more information. Setting the value of the **window** option is the same as using the **−w** command line option. 

If the value of the **window** option (as set by the **window** , **w300** , **w1200** or **w9600** options) is smaller than the actual size of the screen, large screen movements will result in displaying only that smaller number of lines on the screen. (Further movements in that same area will result in the screen being filled.) This can provide a performance improvement when viewing different places in one or more files over a slow link. 

Resetting the window size does not reset the default number of lines scrolled by the **<controlD>** and **<control-U>** commands. 

## **windowname [off]** 

**Vi** changes the name of the editor’s icon/window to possible and not destructive, i.e., when the editor can restore it to its original value on exit or when the icon/window will be discarded as the editor exits. If the **windowname** edit option is set, **vi** will change the icon/window name even when it’s destructive and the icon/window name will remain after the editor exits. (This is the case for _xterm_ (1)). 

## **wraplen, wl [0]** 

This option is identical to the **wrapmargin** ber of columns from the _left_ margin before the line splits, not the right margin. 

If both **wraplen** and **wrapmargin** are set, the **wrapmargin** value is used. 

## **wrapmargin, wm [0]** 

**Vi** only. If the value of the **wrapmargin** option is non-zero, **vi** will split lines so that they end at least that number of columns before the right-hand margin of the screen. (Note, the value of **wrapmargin** is _not_ a text length. In a screen that is 80 columns wide, the command “ `:set wrapmargin=8` ” attempts to keep the lines less than or equal to 72 columns wide.) 

Lines are split at the previous whitespace character closest to the number. Any trailing whitespace characters before that character are deleted. If the line is split because of an inserted `<space>` or `<tab>` character, and you then enter another `<space>` character, it is discarded. 

If wrapmargin is set to 0, or if there is no blank character upon which to split the line, the line is not broken. 

If both **wraplen** and **wrapmargin** are set, the **wrapmargin** value is used. 


**USD:13-66** 

## **wrapscan, ws [on]** 

starting point. Otherwise, the end or beginning of the file terminates the search. 

## **writeany, wa [off]** 

**write** and **xit** commands, or before an automatic write (see the **autowrite** option), are not made. This allows a write to any file, provided the file permissions allow it. 

**USD:13-67** 


## **19. Index** 

|**19. Index**|||||||
|---|---|---|---|---|---|---|
|!||30, 60|@|38, 61|chdir|64|
|""||59|A|38|columns|83|
|#||31, 61|B|38|comment|83|
|$||31|C|39|copy|64|
|%||32|D|39|count|23, 58|
|&||32, 73|E|39|cscope|64|
|(||33|F|40|current pathname|18|
|)||34|G|40|d|46|
|*||61|H|40|delete|64|
|+||27|I|40|directory|83|
|,||34|J|41|display|65|
|/RE/||35|L|41|e|47|
|0||36|M|41|edcompatible|83|
|0<control-D>||54|N|35|edit|65|
|:||36|O|42|errorbells|83|
|;||37|P|42|escapetime|83|
|<||37, 61|Q|42|exrc|83|
|<carriage-return>||20|R|42|extended|84|
|<control-A>||25|S|43|exusage|66|
|<control-B>||25|T|43|f|47|
|<control-D><br><control-E><br><control-F><br><control-G><br><control-H>|25,|54, 60<br>26<br>26<br>26<br>26, 54|U<br>W<br>X<br>Y<br>ZZ|43<br>43<br>44<br>44<br>44|fg<br>fle<br>flec<br>fags<br>fash|66<br>58, 66<br>84<br>58<br>84|
|<control-J>||27|[[|44|global|66|
|<control-L>||27|−|34|hardtabs|84|
|<control-M>||27|]]|45|help|67|
|<control-N>||27|ˆ|45|i|47|
|<control-P>||27|ˆ<control-D>|54|iclower|84|
|<control-R>||27|_|45|ignorecase|85|
|<control-T>||28, 54|‘<character>|33|insert|67|
|<control-U>||28|a|45|j|27|
|<control-W>||28, 55|abbrev|62|join|68|
|<control-X>||55|alternate pathname|18|k|27, 69|
|<control-Y>||28|altwerase|80|keytime|85|
|<control-Z>||29, 74|append|63|l|30|
|<control-]>||29|args|63|leftright|85|
|<control-ˆ>||29|autoindent|80|line|58|
|<end-of-fle>||58, 60|autoprint|81|lines|85|
|<erase>||54|autowrite|81|lisp|85|
|<escape>||29, 55|b|46|list|68, 85|
|<interrupt>|17,|53, 55|backup|81|lock|85|
|<line erase>||55|beautify|82|m|47|
|<literal-next>||17, 54|bg|63|magic|85|
|<newline>||20|bigword|24|map|68|
|<nul>||54|buffer|18|mark|69|
|<space>||30|c|46|matchtime|86|
|<word erase>||55|cd|64|mesg|86|
|=||62|cdpath|82|mkexrc|69|
|>||37, 62|cedit|82|modelines|86|
|?RE?||35|change|64|motion|22|



**USD:13-68** 


|move|69|t|49, 64|
|---|---|---|---|
|msgcat|86|tabstop|91|
|n|35|tag|74|
|next|69|taglength|91|
|noprint|87|tagnext|75|
|number|61, 87|tagpop|75|
|o|48|tagprev|75|
|octal|87|tags|92|
|open|70, 87|tagtop|76|
|optimize|87|term|92|
|p|48|terse|92|
|paragraph|24|tildeop|92|
|paragraphs|87|timeout|92|
|path|88|ttywerase|92|
|preserve|70|u|49|
|previous|70|unabbrev|76|
|previous context|22|undo|76|
|print|71, 88|unmap|76|
|prompt|88|unnamed buffer|19|
|put|71|v|67|
|quit|71|verbose|92|
|r|48|version|76|
|range|58|visual|76, 77|
|read|71|viusage|77|
|readonly|88|w|49|
|recdir|88|w1200|92|
|recover|72|w300|92|
|redraw|89|w9600|93|
|remap|89|warn|93|
|report|89|whitespace|20|
|resize|72|window|93|
|rewind|72|windowname|93|
|ruler|89|wn|77|
|s|48|word|23|
|scroll|90|wq|77|
|searchincr|90|wraplen|93|
|section|24|wrapmargin|94|
|sections|90|wrapscan|94|
|secure|90|write|77|
|sentence|24|writeany|94|
|set|72|x|50|
|shell|73, 90|xit|78|
|shellmeta|90|y|50|
|shiftwidth|91|yank|78|
|showmatch|91|z|50, 78|
|showmode|91|{|51|
|sidescroll|91|||51|
|slowopen|91|}|52|
|source|73|˜|52, 73|
|sourceany|91|||
|stop|74|||
|substitute|73|||
|suspend|74|||



**USD:13-3** 


## **Table of Contents** 

|**Table of Contents**||
|---|---|
|Description ................................................................................................................................|4|
|Additional Features in Nex/Nvi ................................................................................................|4|
|Startup Information ...................................................................................................................|5|
|Recovery ....................................................................................................................................|6|
|Sizing the Screen .......................................................................................................................|7|
|Character Display ......................................................................................................................|7|
|Multiple Screens ........................................................................................................................|7|
|Tags, Tag Stacks, and Cscope ....................................................................................................|8|
|Regular Expressions and Replacement Strings .........................................................................|10|
|Scripting Languages ..................................................................................................................|10|
|General Editor Description ........................................................................................................|12|
|Vi Description ...........................................................................................................................|14|
|Vi Commands ............................................................................................................................|17|
|Vi Text Input Commands ..........................................................................................................|38|
|Ex Addressing ...........................................................................................................................|39|
|Ex Description ...........................................................................................................................|40|
|Ex Commands ...........................................................................................................................|41|
|Set Options ................................................................................................................................|55|
|Index ..........................................................................................................................................|67|



