# FOXDEB web interface
The FOXDEN web interface is simple and intuitive. Here we'll walk you through
its main components.

<br/>

### Login page
When you first time login to FOXDEN server you will be prompted to provide
your CHESS Kerberos credentials. Please note FOXDEN does not store them
anywhere and only obtain a valid kerberos ticket using your credentials and
create associated [token](/docs/tokens.md).

![](/images/foxden_login.pnd)

<br/>

### Main page
The FOXDEN main page contains set of widgets to naviate your through
FOXDEN services. If you are CHESS user who only need to look at
(meta|provenance) data you will only need a **Discovery page**. For other
use cases please navigate to appropriate FOXDEN service.

![](/images/foxden_main.png)

<br/>

### Documentation page
The FOXDEN documentation page is very well organized and provide enough
materials to get you started.

![](/images/foxden_documentation.png)

<br/>

### FOXDEN search
FOXDEN search interface page allows user to search for the favorite FOXDEN
(meta|provenance) data records using either key-value pairs, or fully featured
FOXDEN Query Language (QL). Please use **Show me** button to get more examples.
![](/images/foxden_records.png)

Once you found what you want you will be redirected to FOXDEN records page.
This page may contain one or more records reflected to your search query.

<br/>

#### FOXDEN records
![](/images/foxden_records.png)

<br/>

Each record can be viewed in different forms:
- tabular format to see table like key:value pairs
- desciption format to provide you information about meaning of each key
- and JSON data-format for you to grab and go on

Below you can see examples of each invidual record representation:

<br/>

#### Record tabular format
![](/images/foxden_record.png)

<br/>

#### Record record description
![](/images/foxden_description.png)

<br/>

#### Record JSON format
![](/images/foxden_json.png)
