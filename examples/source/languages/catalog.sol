// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/// @title A tiny on-chain catalog of example files.
contract Catalog {
    struct Entry {
        string name;
        uint256 size;
    }

    Entry[] private entries;

    event Added(string name, uint256 size);

    function add(string calldata name, uint256 size) external {
        entries.push(Entry(name, size));
        emit Added(name, size);
    }

    function total() external view returns (uint256 sum) {
        for (uint256 i = 0; i < entries.length; i++) {
            sum += entries[i].size;
        }
    }
}
