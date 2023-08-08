describe('demo end to end test', () => {
  it('load page', () => {
    cy.visit('/');
    cy.contains('Hello World').should('exist');
  });
});
