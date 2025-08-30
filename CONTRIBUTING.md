# Contributing to Argus

First, thank you for considering contributing to Argus. We appreciate the time and effort you are willing to invest. This document outlines the guidelines for contributing to the project to ensure a smooth and effective process for everyone involved.

## How to Contribute

We welcome contributions in various forms, including:
- Reporting bugs
- Suggesting enhancements
- Improving documentation
- Submitting code changes

### Reporting Bugs

If you encounter a bug, please open an issue on our GitHub repository. A well-documented bug report is crucial for a swift resolution. Please include the following information:

- **Go Version**: The output of `go version`.
- **Operating System**: Your OS and version (e.g., Ubuntu 22.04, macOS 12.6).
- **Clear Description**: A concise but detailed description of the bug.
- **Steps to Reproduce**: A minimal, reproducible example that demonstrates the issue. This could be a small Go program.
- **Expected vs. Actual Behavior**: What you expected to happen and what actually occurred.
- **Logs or Error Messages**: Any relevant logs or error output, formatted as code blocks.

### Suggesting Enhancements

If you have an idea for a new feature or an improvement to an existing one, please open an issue to start a discussion. This allows us to align on the proposal before any significant development work begins.

## Development Process

1.  **Fork the Repository**: Start by forking the main argus repository to your own GitHub account.
2.  **Clone Your Fork**: Clone your forked repository to your local machine.
    ```bash
    git clone https://github.com/YOUR_USERNAME/argus.git
    cd argus
    ```
3.  **Create a Branch**: Create a new branch for your changes. Use a descriptive name (e.g., `fix/....` or `feature/....`).
    ```bash
    git checkout -b your-branch-name
    ```
4.  **Make Changes**: Write your code. Ensure your code adheres to Go's best practices.
5.  **Format Your Code**: Run `gofmt` to ensure your code is correctly formatted.
    ```bash
    gofmt -w .
    ```
6.  **Add Tests**: If you are adding a new feature or fixing a bug, please add corresponding unit or integration tests. All tests must pass.
    ```bash
    go test ./...
    ```
7.  **Commit Your Changes**: Use a clear and descriptive commit message.
    ```bash
    git commit -m "feat: Add support for XYZ feature"
    ```
8.  **Push to Your Fork**: Push your changes to your forked repository.
    ```bash
    git push origin your-branch-name
    ```
9.  **Open a Pull Request**: Open a pull request from your branch to the `main` branch of the official argus repository. Provide a clear title and description for your PR, referencing any related issues.

## Pull Request Guidelines

- **One PR per Feature**: Each pull request should address a single bug or feature.
- **Clear Description**: Explain the "what" and "why" of your changes.
- **Passing Tests**: Ensure that the full test suite passes.
- **Documentation**: If your changes affect public APIs or behavior, update the relevant documentation (in-code comments, `README.md`, or files in the `docs/` directory).

Thank you for helping make Argus better!

---

Argus • an AGILira fragment
