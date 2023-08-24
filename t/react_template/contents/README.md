# React Framework

This template was bootstrapped with [Create React App](https://github.com/facebook/create-react-app). 

The react template enpower the front end development flow by adding automate Continuous Build (CB) and Continuous Testing (CT). GitHub Actions are leveraged to perform a series of pre-submit validations. 

A branch protection rule will be established to enforce checks on each pull request including

- Lint to provide static analysis (with ESlint)
- Code format (with Prettier)
- Build (with React-scripts)
- Unit test and Component test
- Integration test (with Cypress)

## Available Scripts
prerequisites: `node.js` and `npm`. If not, follow the download steps [here](https://docs.npmjs.com/downloading-and-installing-node-js-and-npm).

In the project directory, you can run:

### `npm start`

Runs the app in the development mode. Open [http://localhost:3000](http://localhost:3000) to view it in your browser.

The page will reload when you make changes. You may also see any lint errors in the console.

### `npm test`

Launches the test runner in the interactive watch mode.
See the section about [running tests](https://facebook.github.io/create-react-app/docs/running-tests) for more information.

### `npm run lint`

Apply static analysis and code format checks.

### `npm run fix`

Automatically fix the code problem with Eslint, Prettier and GTS.

### `npm run build`

Builds the app for production to the `build` folder. 

It correctly bundles React in production mode and optimizes the build for the best performance.

The build is minified and the filenames include the hashes. 

Your app is ready to be deployed!

See the section about [deployment](https://facebook.github.io/create-react-app/docs/deployment) for more information.

## Cypress test

Recommend [Cypress Chrome Recorder](https://chrome.google.com/webstore/detail/cypress-chrome-recorder/fellcphjglholofndfmmjmheedhomgin?hl=en), a Chrome extension. It allows exporting tests directly from the Recorder panel.

### `npm run cy:open`

Run cypress cmd to open the cypress launchpad in the browser of the same machine. The running machine should meet [Linux Prerequisites](https://docs.cypress.io/guides/getting-started/installing-cypress#Linux-Prerequisites). For example, if the running is in cloudtop environment, the launchpad should also open on the cloudtop. 

Choose Electron as the target testing browser. Chrome won't work. 

### `npm run cy:test`

Run cypress tests locally. Keep the server running when testing. Make sure to run the test before submit any CL.
