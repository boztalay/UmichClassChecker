UmichClassChecker
=================

A Google App Engine web app in Go to periodically check the availability of classes at the University of Michigan and notify users of changes in availability

http://umichclasschecker.appspot.com

Current version: 0.2.6

Recent Updates
--------------

- Version 0.2.6
	- It now redirects to the login page automatically
	- Replaced the 'Remove' button with an icon
	- Removed 'Action' from the header
	- Removed the school input, turns out it doesn't matter
	- All fields are now required
- Version 0.2.5
	- Added a basic statistics page
	- Minor layout changes
- Version 0.2.4
	- It looks nice now with a CSS overhaul by [nkorth](https://www.github.com/nkorth)!
- Version 0.2.3
	- You can now delete classes!

Improvements to be made
-----------------------

- Change how the data is structured
- Check for duplicate class entries
- Make error handling nicer for users
