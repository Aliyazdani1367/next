# Dashboard UI for Next

## Requirements

For development, you will only need Node.js installed on your environement.

### Node

[Node](http://nodejs.org/) is really easy to install & now include [NPM](https://npmjs.org/).
Use Node.js 20 LTS, which is also used in CI.

## Install

    git clone https://github.com/aliking1367/next.git
    cd Next/dashboard
    npm ci

### Configure app

Copy `example.env` to `.env` then set the backend api address:

    VITE_BASE_API=https://somewhere.com/api/

#### Environment variables

| Name          | Description                                                                          |
| ------------- | ------------------------------------------------------------------------------------ |
| VITE_BASE_API | The api url of the deployed backend ([Next](https://github.com/aliking1367/next)) |

## Start development server

    npm run dev

## Simple build for production

    npm run build

## Tests

    npm test

## Contribution

Feel free to contribute. Go on and fork the project. After commiting the changes, make a PR. It means a lot to us.
