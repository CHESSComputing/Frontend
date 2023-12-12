### FOXDEN tasks

- Ask for some data, and put it to s3, then test fetching this data and streaming.
- Add mongodb to meatada
- Move common data structs from services to common/data
  - for instance, the client and Frontend re-use Site/MetaData structs
  and therefore we need to put them into common place
- move common functions to common/utils or common/tools, e.g.
  - httpGet, httpPost
- Add Google map to main page with site icon, the sites info should come from metadata which should supply geo locations (PARTIALLY DONE]
  - need Google API key for that which requires credit card on file with Google
- Add storage endpoint to create bucket and upload data [PARTIALLY DONE]
- Decide on common icon style and define all images
- Switch to restful endpoints, eg /storage/Cornell/bucket, add http delete, put, post methods to it [DONE]
- add proper cookies and session store, see
  [document](https://stackoverflow.com/questions/66289603/use-existing-session-cookie-in-gin-router)
- Move code from frontend storage handler to data management service. Then, storage handler will call data management api [DONE]
- Add stie registration form page with captcha [DONE]
- Split site menu to submenu: registration, access, storage. The former will lead to site registration page. The second to current sites endpoint, and latter to site s3 [PARTIALLY DONE]
- Add data menu with management, access, viewer submenus. In management page we need form to create datasets , upload files, etc [PARTIALLY DONE]
- Create new repo for foxden client [PARTIALLY DONE]
```
foxden site add ...
foxden site ls ...
foxden meta add....
foxden DBS add...
foxden data create Cornell/bucket
foxden data upload Cornell/bucket file
foxden discover Cornell
```
- In metadata and discover repositories create handlers.go module.
- Add mongodb.go to common repo that we can use in other services.
- Add DBS codebase with simple schema.
- Add common/data where I should store all structures used in different modules to avoid code duplication.
