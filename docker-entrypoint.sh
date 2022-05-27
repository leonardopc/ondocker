#!/bin/sh
set -e
#copy config.json if it does not exist
if [ ! -f "/config/config.json" ]; then
    echo "Creating configuration file..."
    cp /ondocker/config.json "/config/config.json"
fi
#create static folder if it does not exist
if [ ! -d "/config/static" ]; then
    mkdir /config/static
fi
#copy errorPage.html if it does not exist
if [ ! -f "/config/static/errorPage.html" ]; then
    echo "Creating errorPage.html..."
    cp /ondocker/static/errorPage.html "/config/static/errorPage.html"
fi
#copy loadingPage.html if it does not exist
if [ ! -f "/config/static/loadingPage.html" ]; then
    echo "Creating loadingPage.html..."
    cp /ondocker/static/loadingPage.html "/config/static/loadingPage.html"
fi

exec "$@"