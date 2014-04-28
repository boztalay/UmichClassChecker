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
- Version 0.2.2
	- More error logging, I still didn't have enough information
	- Changed the version number on the home page to be templated instead of hardcoded
- Version 0.2.1
	- Made error logging for API requests better so I can see why some requests are failing
	- Added a message to the top of the homepage to let users know which account they're logged in with
	- Added a few links and a version number to the homepage
- Version 0.2
	- Upgraded to use the U of M APIs
	- Changed how often it checks classes from every 15 minutes to every 30 minutes
	- Minor UI changes

Improvements to be made
-----------------------

- Change how the data is structured
- Check for duplicate class entries
- Make error handling nicer for users
